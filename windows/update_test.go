package main

import (
	"os"
	"strings"
	"testing"
)

func TestStoreOnlyUpdateInfo(t *testing.T) {
	app := &App{}
	got, err := app.CheckForUpdates()
	if err != nil {
		t.Fatal(err)
	}
	if got.StoreAvailabilityVerified {
		t.Fatal("unimplemented Store check must not claim availability")
	}
	if got.StoreURL != storeListingURL {
		t.Fatalf("wrong Store target %s", got.StoreURL)
	}
	if got.CurrentVersion == "" {
		t.Fatal("installed/development version is required")
	}
	if strings.Contains(strings.ToLower(got.Note), "github") {
		t.Fatal("update flow must be Store-only")
	}
}

// TestCustomerFacingCopyContainsNoReleaseEngineeringNotes protects the
// customer-facing Settings and Updates UI from unfinished-feature commentary.
// Release blockers and StoreContext availability checks belong in private docs.
func TestCustomerFacingCopyContainsNoReleaseEngineeringNotes(t *testing.T) {
	html, err := os.ReadFile("frontend/dist/index.html")
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{
		"development preview",
		"not yet available",
		"requires testing",
		"not implemented",
		"does not check update availability",
		"mdtranscode core",
	}
	for _, phrase := range forbidden {
		if strings.Contains(strings.ToLower(string(html)), phrase) {
			t.Errorf("customer-facing HTML contains development wording %q", phrase)
		}
	}
	if strings.Contains(strings.ToLower(string(html)), "github") {
		t.Fatal("customer-facing Updates must use Microsoft Store exclusively")
	}
	js, err := os.ReadFile("frontend/dist/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(js), `textContent = "development build"`) {
		t.Fatal("JavaScript fallback must not show development-build wording")
	}
	got, err := (&App{}).CheckForUpdates()
	if err != nil {
		t.Fatal(err)
	}
	for _, phrase := range forbidden {
		if strings.Contains(strings.ToLower(got.Note), phrase) {
			t.Errorf("Store update response contains development wording %q", phrase)
		}
	}
}
