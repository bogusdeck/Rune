package main

import "notes_maker/pkg/core"

type appConfig = core.AppConfig

func defaultAppConfig() appConfig {
	return core.DefaultAppConfig()
}

func appConfigPath() string {
	return core.AppConfigPath("")
}

func loadAppConfig() appConfig {
	return core.LoadAppConfig("")
}

func saveAppConfig(cfg appConfig) {
	core.SaveAppConfig("", cfg)
}

func personalizedContext(profile string, enabled bool) string {
	return core.PersonalizedContext(profile, enabled)
}
