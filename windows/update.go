package main

// Microsoft Store is MDTranscode's sole Windows update destination.
// Availability is deliberately NOT inferred from public GitHub source tags.
// Native StoreContext detection remains a separate packaged-app acceptance task.
const storeListingURL = "https://apps.microsoft.com/detail/9MXCQXBPS8K1"

type UpdateInfo struct {
	CurrentVersion            string `json:"currentVersion"`
	StoreURL                  string `json:"storeUrl"`
	StoreAvailabilityVerified bool   `json:"storeAvailabilityVerified"`
	Note                      string `json:"note"`
}

// CheckForUpdates is kept for Wails binding compatibility, but deliberately
// reports no availability result. The UI only offers the Store destination.
func (a *App) CheckForUpdates() (UpdateInfo, error) {
	return UpdateInfo{
		CurrentVersion:            displayVersion(),
		StoreURL:                  storeListingURL,
		StoreAvailabilityVerified: false,
		Note:                      "Open MDTranscode in Microsoft Store.",
	}, nil
}
