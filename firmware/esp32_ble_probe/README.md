# ESP32 BLE keyboard probe

This sketch targets the original ESP32, including ESP32-WROOM-32 development boards. The USB serial port receives commands and Bluetooth LE delivers the keyboard reports. Pair **Go Fish ESP32 Probe** in Windows Bluetooth settings, then enter the assigned COM port in Go Fish under **Settings → Keys → ESP32 port**.

Build and upload with Arduino CLI and Espressif's Arduino ESP32 core.

```powershell
arduino-cli core update-index --additional-urls https://espressif.github.io/arduino-esp32/package_esp32_index.json
arduino-cli core install esp32:esp32@3.3.12 --additional-urls https://espressif.github.io/arduino-esp32/package_esp32_index.json
arduino-cli compile --fqbn esp32:esp32:esp32 firmware/esp32_ble_probe
arduino-cli upload --fqbn esp32:esp32:esp32 --port COM3 firmware/esp32_ble_probe
```

Replace `COM3` with the port Windows assigns. The upload replaces the existing firmware, so save a flash backup if you need to restore it.

The serial link runs at 115200 baud. Terminate each command with a newline.

| Command | Response |
| --- | --- |
| `PING` | `PONG waiting` before Bluetooth connects, or `PONG connected` afterwards |
| `HELLO` | `GOFISH 1 READY` when Bluetooth is connected, or `GOFISH 1 WAITING` otherwise |
| `TAP 04` | Queues the HID usage in hexadecimal, holds it for 40 ms, and queues its release. Returns `OK` when the BLE stack accepts both reports, or an `ERR` response on failure. |
| `TEST` | Performs the same tap as `TAP 04`. |

Focus a blank Notepad window before sending `TEST`. The serial response describes the board's state. Check the editor to confirm Windows received the key.
