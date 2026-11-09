#include <Arduino.h>
#include <BLEDevice.h>
#include <BLEHIDDevice.h>
#include <BLESecurity.h>
#include <Preferences.h>
#include <WebServer.h>
#include <WiFi.h>
#include <atomic>
#include <cstdlib>
#include <cstring>

namespace {
constexpr char deviceName[] = "Go Fish ESP32 Probe";
constexpr uint8_t keyboardReportID = 1;
constexpr uint8_t mouseReportID = 2;
constexpr uint8_t keyA = 0x04;
constexpr uint8_t rightButton = 0x02;
constexpr unsigned long holdMs = 40;
constexpr unsigned long clickMs = 30; // same hold as the app's Windows right click

// Report 1, keyboard: modifiers, reserved byte, six key usages.
// Report 2, mouse: three buttons, then relative X and Y from -127 to 127.
uint8_t reportMap[] = {
    0x05, 0x01, 0x09, 0x06, 0xa1, 0x01, 0x85, keyboardReportID,
    0x05, 0x07, 0x19, 0xe0, 0x29, 0xe7, 0x15, 0x00, 0x25, 0x01,
    0x75, 0x01, 0x95, 0x08, 0x81, 0x02,
    0x75, 0x08, 0x95, 0x01, 0x81, 0x01,
    0x05, 0x07, 0x19, 0x00, 0x29, 0x65, 0x15, 0x00, 0x25, 0x65,
    0x75, 0x08, 0x95, 0x06, 0x81, 0x00, 0xc0,
    0x05, 0x01, 0x09, 0x02, 0xa1, 0x01, 0x85, mouseReportID,
    0x09, 0x01, 0xa1, 0x00,
    0x05, 0x09, 0x19, 0x01, 0x29, 0x03, 0x15, 0x00, 0x25, 0x01,
    0x95, 0x03, 0x75, 0x01, 0x81, 0x02,
    0x95, 0x01, 0x75, 0x05, 0x81, 0x03,
    0x05, 0x01, 0x09, 0x30, 0x09, 0x31, 0x15, 0x81, 0x25, 0x7f,
    0x75, 0x08, 0x95, 0x02, 0x81, 0x06, 0xc0, 0xc0,
};

std::atomic<bool> connected{false};
BLECharacteristic *keyboardInput;
BLECharacteristic *mouseInput;
bool reportQueued;
char line[104]; // fits WIFI with a 32-byte SSID and 63-byte password
size_t length;
bool invalidLine;

// Ring buffer of the last serial commands, shown on the status page.
struct Entry {
  unsigned long ms;
  char command[16];
  char input[16]; // key name, "right click", or the number of merged moves
  const char *reply;
  uint16_t moves; // consecutive MOVE reports merged into this row
  int dx, dy;     // their sum
};
constexpr size_t logSize = 64;
Entry entries[logSize];
size_t logCount; // rows since boot; the newest is entries[(logCount - 1) % logSize]
size_t inputsOK;
size_t inputsFailed;

class ConnectionCallbacks : public BLEServerCallbacks {
  void onConnect(BLEServer *) override { connected = true; }
  void onDisconnect(BLEServer *) override { connected = false; }
};

ConnectionCallbacks connectionCallbacks;

class ReportCallbacks : public BLECharacteristicCallbacks {
  void onStatus(BLECharacteristic *, Status status, uint32_t) override {
    reportQueued = status == Status::SUCCESS_NOTIFY;
  }
};

ReportCallbacks reportCallbacks;

bool notify(BLECharacteristic *c, uint8_t *report, size_t size) {
  c->setValue(report, size);
  reportQueued = false;
  c->notify();
  return reportQueued;
}

bool sendKey(uint8_t usage) {
  uint8_t report[8] = {};
  report[2] = usage;
  return notify(keyboardInput, report, sizeof(report));
}

bool sendMouse(uint8_t buttons, int dx, int dy) {
  uint8_t report[3] = {buttons, static_cast<uint8_t>(dx), static_cast<uint8_t>(dy)};
  return notify(mouseInput, report, sizeof(report));
}

bool parseUsage(uint8_t &usage) {
  if (length != 6 || std::strncmp(line, "TAP ", 4) != 0) {
    return false;
  }
  for (size_t i = 4; i < 6; ++i) {
    const char ch = line[i];
    if (ch >= '0' && ch <= '9') {
      usage = (usage << 4) | (ch - '0');
    } else if (ch >= 'A' && ch <= 'F') {
      usage = (usage << 4) | (ch - 'A' + 10);
    } else {
      return false;
    }
  }
  return usage >= 0x04 && usage <= 0x65;
}

// parseMove reads "MOVE <dx> <dy>" with both values from -127 to 127.
bool parseMove(int &dx, int &dy) {
  if (std::strncmp(line, "MOVE ", 5) != 0) {
    return false;
  }
  char *end;
  const long x = std::strtol(line + 5, &end, 10);
  if (end == line + 5 || *end != ' ') {
    return false;
  }
  const char *start = end + 1;
  const long y = std::strtol(start, &end, 10);
  if (end == start || *end != '\0' || x < -127 || x > 127 || y < -127 || y > 127) {
    return false;
  }
  dx = x;
  dy = y;
  return true;
}

const char *result(bool ok) { return ok && connected ? "OK" : "ERR BLE report"; }

const char *tap(uint8_t usage) {
  if (!connected) {
    return "ERR disconnected";
  }
  const bool down = sendKey(usage);
  delay(holdMs);
  return result(sendKey(0) && down);
}

const char *click() {
  if (!connected) {
    return "ERR disconnected";
  }
  const bool down = sendMouse(rightButton, 0, 0);
  delay(clickMs);
  return result(sendMouse(0, 0, 0) && down);
}

const char *move(int dx, int dy) {
  return connected ? result(sendMouse(0, dx, dy)) : "ERR disconnected";
}

// Readable name of a HID keyboard usage, its hex value when it has none.
String keyName(uint8_t u) {
  if (u >= 0x04 && u <= 0x1d) {
    return String(char('A' + u - 0x04));
  }
  if (u >= 0x1e && u <= 0x27) {
    return String(char(u == 0x27 ? '0' : '1' + u - 0x1e));
  }
  if (u >= 0x3a && u <= 0x45) {
    return "F" + String(u - 0x3a + 1);
  }
  switch (u) {
  case 0x28: return "Enter";
  case 0x29: return "Esc";
  case 0x2a: return "Backspace";
  case 0x2b: return "Tab";
  case 0x2c: return "Space";
  default: return "key " + String(u, HEX);
  }
}

// record logs the current line. input is empty for commands that send nothing.
// Consecutive MOVE commands with the same reply merge into one row, so a
// pointer path does not push the key presses off the page.
void record(const char *reply, const String &input, bool isMove, int dx, int dy) {
  if (!input.isEmpty()) {
    ++(reply[0] == 'O' ? inputsOK : inputsFailed);
  }
  Entry *e = logCount ? &entries[(logCount - 1) % logSize] : nullptr;
  if (isMove && e && e->moves && e->reply == reply) {
    ++e->moves;
    e->dx += dx;
    e->dy += dy;
  } else {
    e = &entries[logCount++ % logSize];
    e->moves = isMove;
    e->dx = dx;
    e->dy = dy;
    e->reply = reply;
    snprintf(e->input, sizeof(e->input), "%s", input.c_str());
  }
  e->ms = millis();
  if (isMove) {
    snprintf(e->command, sizeof(e->command), "MOVE %+d %+d", e->dx, e->dy);
    snprintf(e->input, sizeof(e->input), "%u move%s", e->moves, e->moves > 1 ? "s" : "");
    return;
  }
  size_t i = 0;
  for (; i < sizeof(e->command) - 1 && line[i] != '\0'; ++i) {
    // Keep the page free of markup whatever arrives on the serial port.
    e->command[i] = isalnum(line[i]) || line[i] == ' ' ? line[i] : '?';
  }
  e->command[i] = '\0';
}

void handleWiFi();

void handleLine() {
  if (!invalidLine && std::strncmp(line, "WIFI", 4) == 0) {
    handleWiFi(); // typed by a person, not Go Fish, so not logged
    return;
  }
  uint8_t usage = 0;
  int dx = 0, dy = 0;
  bool isMove = false;
  const char *reply;
  String input;
  if (invalidLine) {
    reply = "ERR command";
  } else if (std::strcmp(line, "PING") == 0) {
    reply = connected ? "PONG connected" : "PONG waiting";
  } else if (std::strcmp(line, "HELLO") == 0) {
    reply = connected ? "GOFISH 2 READY" : "GOFISH 2 WAITING";
  } else if (std::strcmp(line, "TEST") == 0) {
    reply = tap(keyA);
    input = keyName(keyA);
  } else if (parseUsage(usage)) {
    reply = tap(usage);
    input = keyName(usage);
  } else if (std::strcmp(line, "CLICK") == 0) {
    reply = click();
    input = "right click";
  } else if (parseMove(dx, dy)) {
    reply = move(dx, dy);
    input = "move";
    isMove = true;
  } else {
    reply = "ERR command";
  }
  Serial.println(reply);
  record(reply, input, isMove, dx, dy);
}

WebServer http(80);

String statusJSON() {
  char buf[220];
  snprintf(buf, sizeof(buf),
           "{\"uptime_s\":%lu,\"ble_connected\":%s,\"rows\":%u,\"inputs_ok\":%u,"
           "\"inputs_failed\":%u,\"free_heap\":%u,\"rssi\":%d}",
           millis() / 1000, connected ? "true" : "false", logCount, inputsOK, inputsFailed,
           ESP.getFreeHeap(), WiFi.RSSI());
  return buf;
}

// Calls row(number, entry, seconds ago) for each logged command, newest first.
template <typename F> void eachEntry(F row) {
  const unsigned long now = millis();
  const size_t n = logCount < logSize ? logCount : logSize;
  for (size_t i = 0; i < n; ++i) {
    const Entry &e = entries[(logCount - 1 - i) % logSize];
    row(logCount - i, e, (now - e.ms) / 1000.0f);
  }
}

String logJSON() {
  String out = "[";
  eachEntry([&](size_t n, const Entry &e, float ago) {
    char buf[160];
    snprintf(buf, sizeof(buf),
             "%s{\"n\":%u,\"ago_s\":%.1f,\"command\":\"%s\",\"input\":\"%s\",\"reply\":\"%s\"}",
             out.length() > 1 ? "," : "", n, ago, e.command, e.input, e.reply);
    out += buf;
  });
  return out + "]";
}

String page() {
  // shortcut: 2 s meta refresh instead of server-sent events, keeps Wi-Fi traffic low.
  String out = "<!doctype html><meta http-equiv=refresh content=2><title>Go Fish ESP32</title>"
               "<style>body{font:14px monospace;margin:16px}"
               "td,th{padding:2px 12px 2px 0;text-align:left}.e{color:#c00}</style><p>";
  out += statusJSON();
  out += "<table><tr><th>#<th>ago<th>command<th>input<th>reply";
  eachEntry([&](size_t n, const Entry &e, float ago) {
    char buf[180];
    snprintf(buf, sizeof(buf), "<tr%s><td>%u<td>%.1fs<td>%s<td>%s<td>%s",
             e.reply[0] == 'E' ? " class=e" : "", n, ago, e.command, e.input, e.reply);
    out += buf;
  });
  return out + "</table>";
}

// Open page on purpose: the board is meant for a trusted home LAN.
void startWiFi() {
  Preferences prefs;
  prefs.begin("wifi", true);
  const String ssid = prefs.getString("ssid");
  const String pass = prefs.getString("pass");
  prefs.end();
  if (ssid.isEmpty()) {
    return;
  }
  WiFi.mode(WIFI_STA);
  WiFi.setSleep(true); // modem sleep is mandatory while BLE shares the radio
  WiFi.begin(ssid, pass);
  http.on("/", [] { http.send(200, "text/html", page()); });
  http.on("/status", [] { http.send(200, "application/json", statusJSON()); });
  http.on("/log", [] { http.send(200, "application/json", logJSON()); });
  http.begin();
}

// WIFI reports the address; WIFI <ssid> <password> stores credentials and
// restarts; WIFI OFF clears them. The last space splits SSID from password, so
// SSIDs may contain spaces. Replies only on request: the app reads one line per command.
void handleWiFi() {
  if (line[4] == '\0') {
    if (WiFi.status() == WL_CONNECTED) {
      Serial.println("WIFI http://" + WiFi.localIP().toString());
    } else {
      Serial.println(WiFi.getMode() == WIFI_OFF ? "WIFI off" : "WIFI connecting");
    }
    return;
  }
  if (line[4] != ' ') {
    Serial.println("ERR command");
    return;
  }
  const char *args = line + 5;
  Preferences prefs;
  prefs.begin("wifi", false);
  if (std::strcmp(args, "OFF") == 0) {
    prefs.clear();
  } else {
    const char *split = std::strrchr(args, ' ');
    if (split == nullptr || split == args || std::strlen(split + 1) < 8) {
      prefs.end();
      Serial.println("ERR WIFI <ssid> <password>");
      return;
    }
    prefs.putString("ssid", String(args).substring(0, split - args));
    prefs.putString("pass", split + 1);
  }
  prefs.end();
  Serial.println("OK restarting");
  Serial.flush();
  ESP.restart();
}

void serveWiFi() {
  if (WiFi.getMode() != WIFI_OFF) {
    http.handleClient();
  }
}
} // namespace

void setup() {
  Serial.begin(115200);
  BLEDevice::init(deviceName);
  BLESecurity::setCapability(ESP_IO_CAP_NONE);
  BLESecurity::setAuthenticationMode(true, false, true);

  BLEServer *server = BLEDevice::createServer();
  server->setCallbacks(&connectionCallbacks);
  server->advertiseOnDisconnect(true);
  auto *hid = new BLEHIDDevice(server);
  keyboardInput = hid->inputReport(keyboardReportID);
  keyboardInput->setCallbacks(&reportCallbacks);
  mouseInput = hid->inputReport(mouseReportID);
  mouseInput->setCallbacks(&reportCallbacks);
  hid->manufacturer()->setValue("Go Fish");
  hid->hidInfo(0, 0x01);
  hid->reportMap(reportMap, sizeof(reportMap));
  hid->startServices();
  hid->setBatteryLevel(100);

  BLEAdvertising *advertising = server->getAdvertising();
  advertising->setAppearance(0x03c0); // generic HID: keyboard and mouse
  advertising->addServiceUUID(hid->hidService()->getUUID());
  advertising->setScanResponse(true);
  advertising->setMinPreferred(0x06);
  advertising->setMaxPreferred(0x12);
  advertising->start();
  startWiFi();
  Serial.println("READY GoFish BLE probe");
}

void loop() {
  while (Serial.available()) {
    const int ch = Serial.read();
    if (ch == '\r') {
      continue;
    }
    if (ch == '\n') {
      line[length] = '\0';
      handleLine();
      length = 0;
      invalidLine = false;
      continue;
    }
    if (ch < 0x20 || ch > 0x7e || length == sizeof(line) - 1) {
      invalidLine = true;
    } else if (!invalidLine) {
      line[length++] = static_cast<char>(ch);
    }
  }
  serveWiFi();
  delay(1);
}
