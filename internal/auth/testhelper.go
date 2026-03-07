package auth

// SetConfigDirForTest overrides the config directory. Call ResetConfigDir to
// restore the default. Intended for use in tests only.
func SetConfigDirForTest(dir string) {
	configDirFunc = func() string { return dir }
}

// ResetConfigDir restores the default config directory function.
func ResetConfigDir() {
	configDirFunc = defaultConfigDir
}
