# ESP32 BLE keyboard and mouse

This sketch targets the original ESP32, including ESP32-WROOM-32 development boards. The USB serial port receives commands and Bluetooth LE delivers keyboard and mouse reports. Pair **Go Fish ESP32 Probe** in Windows Bluetooth settings, then enter the assigned COM port in Go Fish under **Settings → Keys → ESP32 port**.

Build and upload with Arduino CLI and Espressif's Arduino ESP32 core.

```powershell
arduino-cli core update-index --additional-urls https://espressif.github.io/arduino-esp32/package_esp32_index.json
arduino-cli core install esp32:esp32@3.3.12 --additional-urls https://espressif.github.io/arduino-esp32/package_esp32_index.json
arduino-cli compile --fqbn esp32:esp32:esp32 firmware/esp32_ble_probe
arduino-cli upload --fqbn esp32:esp32:esp32 --port COM3 firmware/esp32_ble_probe
```

Replace `COM3` with the port Windows assigns. The upload replaces the existing firmware, so save a flash backup if you need to restore it.

After upgrading from keyboard-only firmware, remove the device from Windows
Bluetooth settings and pair it again so Windows reads the new mouse report.
Set **Reposition key** to `auto` in Go Fish and keep the pointer inside the game
window to allow camera drags. With a keyboard-only board, reposition uses WASD.

The serial link runs at 115200 baud. Terminate each command with a newline.

| Command | Response |
| --- | --- |
| `PING` | `PONG waiting` before Bluetooth connects, or `PONG connected` afterwards |
| `HELLO` | `GOFISH 1 READY` when Bluetooth is connected, or `GOFISH 1 WAITING` otherwise |
| `TAP 04` | Queues the HID usage in hexadecimal, holds it for 40 ms, and queues its release. Returns `OK` when the BLE stack accepts both reports, or an `ERR` response on failure. |
| `TAPMS` | Returns `TAPMS` on firmware that accepts explicit hold durations. |
| `TAP 04 85` | Holds the key for 85 ms. The optional hold is accepted from 10 to 500 ms. |
| `MOUSE` | Returns `MOUSE` on firmware that supports camera drags. |
| `DRAG -12` | Holds the left mouse button, moves horizontally by 12 counts to the left, and releases. Accepts signed nonzero counts from -24 to 24. |
| `TEST` | Performs the same tap as `TAP 04`. |

Focus a blank Notepad window before sending `TEST`. The serial response describes the board's state. Check the editor to confirm Windows received the key.
