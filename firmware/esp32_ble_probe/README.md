# ESP32 BLE keyboard and mouse probe

This sketch targets the original ESP32, including ESP32-WROOM-32 development boards. The USB serial port receives commands and Bluetooth LE delivers keyboard and mouse reports. Pair **Go Fish ESP32 Probe** in Windows Bluetooth settings, then enter the assigned COM port in Go Fish under **Settings → Keys → ESP32 port**.

Build and upload with Arduino CLI and Espressif's Arduino ESP32 core.

```powershell
arduino-cli core update-index --additional-urls https://espressif.github.io/arduino-esp32/package_esp32_index.json
arduino-cli core install esp32:esp32@3.3.12 --additional-urls https://espressif.github.io/arduino-esp32/package_esp32_index.json
arduino-cli compile --fqbn esp32:esp32:esp32:PartitionScheme=huge_app firmware/esp32_ble_probe
arduino-cli upload --fqbn esp32:esp32:esp32:PartitionScheme=huge_app --port COM3 firmware/esp32_ble_probe
```

Replace `COM3` with the port Windows assigns. The upload replaces the existing firmware, so save a flash backup if you need to restore it. BLE and Wi-Fi together outgrow the default 1.3 MB app partition, so the build uses `huge_app` (3 MB app, no OTA).

Windows keeps the input reports a paired board described. After flashing firmware that adds or changes a report, such as the mouse, remove **Go Fish ESP32 Probe** in Bluetooth settings and pair it again.

The serial link runs at 115200 baud. Terminate each command with a newline.

| Command | Response |
| --- | --- |
| `PING` | `PONG waiting` before Bluetooth connects, or `PONG connected` afterwards |
| `HELLO` | `GOFISH 2 READY` when Bluetooth is connected, or `GOFISH 2 WAITING` otherwise. Version 2 has the mouse; Go Fish refuses version 1. |
| `TAP 04` | Queues the HID usage in hexadecimal, holds it for 40 ms, and queues its release. Returns `OK` when the BLE stack accepts both reports, or an `ERR` response on failure. |
| `TEST` | Performs the same tap as `TAP 04`. |
| `MOVE <dx> <dy>` | Sends one relative mouse report, each value from -127 to 127. Windows scales it by its pointer speed, so Go Fish reads the cursor after each one and corrects until it is on the target. Returns `OK` or an `ERR` response. |
| `CLICK` | Presses the right mouse button, holds it for 30 ms, and releases it. Returns `OK` or an `ERR` response. |
| `WIFI <ssid> <password>` | Stores the Wi-Fi credentials in flash and restarts. The last space separates SSID and password. Replies `OK restarting`, or `ERR WIFI <ssid> <password>` when the password is shorter than 8 characters. |
| `WIFI` | `WIFI http://<ip>` once connected, `WIFI connecting` before that, or `WIFI off` without stored credentials |
| `WIFI OFF` | Clears the stored credentials and restarts with Wi-Fi off. |

Focus a blank Notepad window before sending `TEST`. The serial response describes the board's state. Check the editor to confirm Windows received the key.

## Status page

With Wi-Fi credentials stored, the board joins the network on every start. Send `WIFI` to get its address. It serves three pages on port 80:

- `/` shows the status and the last 64 commands from the serial port, newest first, and refreshes every 2 seconds. Each row has the command, what it sent (the key name such as `2` for `TAP 1F`, `right click`, or `5 moves`), the reply the board sent back, and how many seconds ago it arrived. Consecutive `MOVE` commands merge into one row with their summed distance, such as `MOVE -312 +88`. Failed commands are red.
- `/status` returns `uptime_s`, `ble_connected`, `rows` (log rows since boot), `inputs_ok`, `inputs_failed` (taps, clicks and moves), `free_heap` and `rssi` as JSON.
- `/log` returns the rows as JSON: `n`, `ago_s`, `command`, `input`, `reply`.

`WIFI` commands are not logged, so the stored password never shows up on the page. The log lives in RAM and resets when the board restarts.

The pages have no password, so anyone on the LAN can read them. They expose status only and cannot send input. Wi-Fi and BLE share one radio, and BLE timing can wobble while Wi-Fi is busy, so keep the page closed during runs that need tight timing.
