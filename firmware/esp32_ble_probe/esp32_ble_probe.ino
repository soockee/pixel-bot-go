#include <Arduino.h>
#include <BLEDevice.h>
#include <BLEHIDDevice.h>
#include <BLESecurity.h>
#include <atomic>
#include <cstring>

namespace {
constexpr char deviceName[] = "Go Fish ESP32 Probe";
constexpr uint8_t keyboardReportID = 1;
constexpr uint8_t mouseReportID = 2;
constexpr int maxDrag = 24;
constexpr uint8_t keyA = 0x04;
constexpr unsigned long holdMs = 40;

// Eight-byte keyboard report: modifiers, reserved byte, six key usages.
uint8_t reportMap[] = {
    0x05, 0x01, 0x09, 0x06, 0xa1, 0x01, 0x85, keyboardReportID,
    0x05, 0x07, 0x19, 0xe0, 0x29, 0xe7, 0x15, 0x00, 0x25, 0x01,
    0x75, 0x01, 0x95, 0x08, 0x81, 0x02,
    0x75, 0x08, 0x95, 0x01, 0x81, 0x01,
    0x05, 0x07, 0x19, 0x00, 0x29, 0x65, 0x15, 0x00, 0x25, 0x65,
    0x75, 0x08, 0x95, 0x06, 0x81, 0x00, 0xc0,
    // Mouse: buttons, signed relative X and Y.
    0x05, 0x01, 0x09, 0x02, 0xa1, 0x01, 0x85, mouseReportID,
    0x09, 0x01, 0xa1, 0x00,
    0x05, 0x09, 0x19, 0x01, 0x29, 0x03, 0x15, 0x00, 0x25, 0x01,
    0x75, 0x01, 0x95, 0x03, 0x81, 0x02,
    0x75, 0x05, 0x95, 0x01, 0x81, 0x03,
    0x05, 0x01, 0x09, 0x30, 0x09, 0x31, 0x15, 0x81, 0x25, 0x7f,
    0x75, 0x08, 0x95, 0x02, 0x81, 0x06, 0xc0, 0xc0,
};

std::atomic<bool> connected{false};
BLECharacteristic *keyboardInput;
BLECharacteristic *mouseInput;
bool reportQueued;
char line[16];
size_t length;
bool invalidLine;

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

bool notifyReport(BLECharacteristic *input, uint8_t *report, size_t size) {
  input->setValue(report, size);
  reportQueued = false;
  input->notify();
  return reportQueued;
}

bool sendKey(uint8_t usage) {
  uint8_t report[8] = {};
  report[2] = usage;
  return notifyReport(keyboardInput, report, sizeof(report));
}

bool sendMouse(uint8_t buttons, int dx) {
  uint8_t report[] = {buttons, static_cast<uint8_t>(dx), 0};
  return notifyReport(mouseInput, report, sizeof(report));
}

void dragMouse() {
  size_t i = 5;
  const bool negative = line[i] == '-';
  if (negative) {
    ++i;
  }
  int dx = 0;
  for (; i < length; ++i) {
    if (line[i] < '0' || line[i] > '9') {
      Serial.println("ERR command");
      return;
    }
    dx = dx * 10 + (line[i] - '0');
    if (dx > maxDrag) {
      Serial.println("ERR command");
      return;
    }
  }
  if (dx == 0) {
    Serial.println("ERR command");
    return;
  }
  if (!connected) {
    Serial.println("ERR disconnected");
    return;
  }
  const bool down = sendMouse(1, 0);
  delay(25);
  const bool moved = down && connected && sendMouse(1, negative ? -dx : dx);
  delay(25);
  bool up = sendMouse(0, 0);
  if (!up && connected) {
    up = sendMouse(0, 0);
  }
  Serial.println(down && moved && up && connected ? "OK" : "ERR BLE report");
}

bool parseTap(uint8_t &usage, unsigned long &duration) {
  if (length < 6 || std::strncmp(line, "TAP ", 4) != 0) {
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
  if (usage < 0x04 || usage > 0x65) {
    return false;
  }
  if (length == 6) {
    return true;
  }
  if (line[6] != ' ' || length < 9 || length > 10) {
    return false;
  }
  duration = 0;
  for (size_t i = 7; i < length; ++i) {
    if (line[i] < '0' || line[i] > '9') {
      return false;
    }
    duration = duration * 10 + (line[i] - '0');
  }
  return duration >= 10 && duration <= 500;
}

void handleLine() {
  uint8_t usage = 0;
  unsigned long duration = holdMs;
  if (invalidLine) {
    Serial.println("ERR command");
  } else if (std::strcmp(line, "PING") == 0) {
    Serial.println(connected ? "PONG connected" : "PONG waiting");
  } else if (std::strcmp(line, "HELLO") == 0) {
    Serial.println(connected ? "GOFISH 1 READY" : "GOFISH 1 WAITING");
  } else if (std::strcmp(line, "TAPMS") == 0) {
    Serial.println("TAPMS");
  } else if (std::strcmp(line, "MOUSE") == 0) {
    Serial.println("MOUSE");
  } else if (std::strncmp(line, "DRAG ", 5) == 0) {
    dragMouse();
    return;
  } else if (std::strcmp(line, "TEST") == 0) {
    usage = keyA;
  } else if (!parseTap(usage, duration)) {
    Serial.println("ERR command");
    return;
  }
  if (usage == 0) {
    return;
  }
  if (!connected) {
    Serial.println("ERR disconnected");
  } else {
    const bool down = sendKey(usage);
    delay(duration);
    const bool up = sendKey(0);
    Serial.println(down && up && connected ? "OK" : "ERR BLE report");
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
  advertising->setAppearance(0x03c0); // Generic HID, keyboard and mouse
  advertising->addServiceUUID(hid->hidService()->getUUID());
  advertising->setScanResponse(true);
  advertising->setMinPreferred(0x06);
  advertising->setMaxPreferred(0x12);
  advertising->start();
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
  delay(1);
}
