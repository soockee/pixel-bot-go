package app

import "github.com/soocke/pixel-bot-go/internal/esp32"

// findBoard opens the ESP32 on port, or finds it with esp32.AutoPort, which
// checks the firmware and its Bluetooth link without sending input, then
// closes it again and returns the port that answered.
func findBoard(port string) (string, error) {
	var keyboard esp32.Keyboard
	if err := keyboard.Open(port); err != nil {
		return "", err
	}
	defer keyboard.Close()
	return keyboard.Name(), nil
}
