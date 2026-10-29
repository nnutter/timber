package config

// Settings holds the persistent Timber configuration.
type Settings struct {
	AutoMiseTrust        bool `toml:"auto_mise_trust"`
	AutoMiseTrustDecided bool `toml:"auto_mise_trust_decided,omitempty"`
	AutoHKInstall        bool `toml:"auto_hk_install"`
	AutoHKInstallDecided bool `toml:"auto_hk_install_decided,omitempty"`
}
