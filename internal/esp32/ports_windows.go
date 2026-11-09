package esp32

import (
	"cmp"
	"errors"
	"slices"

	"golang.org/x/sys/windows/registry"
)

// Ports lists the COM ports Windows has now, COM3 before COM10.
func Ports() ([]string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DEVICEMAP\SERIALCOMM`, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil, nil // Windows removes the key while no port exists
	}
	if err != nil {
		return nil, err
	}
	defer k.Close()
	names, err := k.ReadValueNames(0)
	if err != nil {
		return nil, err
	}
	var ports []string
	for _, n := range names {
		v, _, err := k.GetStringValue(n)
		if p, perr := PortName(v); err == nil && perr == nil && p != "" {
			ports = append(ports, p)
		}
	}
	slices.SortFunc(ports, func(a, b string) int { return cmp.Or(cmp.Compare(len(a), len(b)), cmp.Compare(a, b)) })
	return slices.Compact(ports), nil
}
