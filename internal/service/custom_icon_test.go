package service

import (
	"testing"

	"github.com/bitxeno/atvloadly/internal/model"
)

func TestSaveAppKeepsAndClearsCustomIcon(t *testing.T) {
	setupTestDB(t)
	app := model.InstalledApp{UDID: "DEVICE", BundleIdentifier: "com.example.app", Account: "user@example.com", CustomIcon: "data:image/png;base64,TEST"}
	saved, err := SaveApp(app)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := GetApp(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.CustomIcon != app.CustomIcon {
		t.Fatal("custom icon was not persisted for refresh")
	}
	app.CustomIcon = "replacement"
	saved, err = SaveApp(app)
	if err != nil {
		t.Fatal(err)
	}
	stored, err = GetApp(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.CustomIcon != "replacement" {
		t.Fatal("replacement icon was not persisted")
	}
	app.CustomIcon = ""
	saved, err = SaveApp(app)
	if err != nil {
		t.Fatal(err)
	}
	stored, err = GetApp(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.CustomIcon != "" {
		t.Fatal("original icon choice did not clear the override")
	}
}
