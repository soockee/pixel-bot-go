package action

import "log/slog"

// DryRun returns input callbacks that only log the requested action instead
// of injecting it. Use them to run the bot against a fake or real window
// without Administrator rights and without sending any input.
func DryRun(logger *slog.Logger) (pressKey func(vk byte) bool, moveCursor func(x, y int), clickRight func() bool) {
	if logger == nil {
		logger = slog.Default()
	}
	pressKey = func(vk byte) bool {
		logger.Info("dry-run action", "action", "press_key", "vk", vk)
		return true
	}
	moveCursor = func(x, y int) { logger.Info("dry-run action", "action", "move_cursor", "x", x, "y", y) }
	clickRight = func() bool {
		logger.Info("dry-run action", "action", "click_right")
		return true
	}
	return
}
