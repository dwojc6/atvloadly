package model

import "testing"

func TestParseDeviceClassWithModelName(t *testing.T) {
	for _, tt := range []struct{ name, class string }{
		{"Davids-iPhone-17-Pro", string(DeviceClassiPhone)},
		{"Alice's IPHONE", string(DeviceClassiPhone)},
		{"iPad-Pro", string(DeviceClassiPad)},
		{"Living-Room", string(DeviceClassAppleTV)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			device := Device{Name: tt.name}
			device.ParseDeviceClass()
			if device.DeviceClass != tt.class {
				t.Errorf("class %q, want %q", device.DeviceClass, tt.class)
			}
		})
	}
	device := Device{Name: "An iPhone named AppleTV", DeviceClass: string(DeviceClassiPad)}
	device.ParseDeviceClass()
	if device.DeviceClass != string(DeviceClassiPad) {
		t.Fatal("explicit device class was overwritten")
	}
}
