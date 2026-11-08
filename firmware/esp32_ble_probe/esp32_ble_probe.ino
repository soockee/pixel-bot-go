#include <Arduino.h>
#include <BLEDevice.h>
#include <BLEHIDDevice.h>
#include <BLESecurity.h>
#include <atomic>
#include <cstring>

namespace {
constexpr char deviceName[] = "Go Fish ESP32 Probe";
constexpr uint8_t keyboardReportID = 1;
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
};

std::atomic<bool> connected{false};
BLECharacteristic *keyboardInput;
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

bool sendReport(uint8_t usage) {
  uint8_t report[8] = {};
  report[2] = usage;
  keyboardInput->setValue(report, sizeof(report));
  reportQueued = false;
  keyboardInput->notify();
  return reportQueued;
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
    const bool down = sendReport(usage);
    delay(duration);
    const bool up = sendReport(0);
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
  hid->manufacturer()->setValue("Go Fish");
  hid->hidInfo(0, 0x01);
  hid->reportMap(reportMap, sizeof(reportMap));
  hid->startServices();
  hid->setBatteryLevel(100);

  BLEAdvertising *advertising = server->getAdvertising();
  advertising->setAppearance(0x03c1); // Keyboard
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
