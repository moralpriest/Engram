package main

import (
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/DEROFDN/engram/i18n"
)

// initLanguage selects the active UI language at startup:
//  1. a previously saved setting wins,
//  2. otherwise the OS locale is detected (first-run convenience) so a
//     Spanish macOS, for example, does not start in English.
func initLanguage() {
	if langData, err := GetValue("settings", []byte("language")); err == nil && len(langData) > 0 {
		i18n.SetLanguage(string(langData))
		return
	}
	if lang := detectSystemLanguage(); lang != "" {
		i18n.SetLanguage(lang)
	}
}

// detectSystemLanguage returns the user's OS language as an Engram i18n code,
// or "" when it cannot be determined or is unsupported.
func detectSystemLanguage() string {
	var raw []string
	switch runtime.GOOS {
	case "darwin":
		// macOS GUI apps do not reliably set LANG; AppleLanguages is authoritative.
		if out, err := exec.Command("/usr/bin/defaults", "read", "-g", "AppleLanguages").Output(); err == nil {
			re := regexp.MustCompile(`"([A-Za-z]{2,3}(?:[-_][A-Za-z0-9]+)*)"|(?i)\b([A-Za-z]{2,3})\b`)
			for _, m := range re.FindAllStringSubmatch(string(out), -1) {
				tok := m[1]
				if tok == "" {
					tok = m[2]
				}
				if tok != "" {
					raw = append(raw, tok)
				}
			}
		}
	default:
		for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANG", "LANGUAGE"} {
			if v := os.Getenv(env); v != "" {
				raw = append(raw, strings.SplitN(v, ".", 2)[0])
			}
		}
	}

	supported := i18n.AvailableLanguages()
	for _, r := range raw {
		primary := strings.ToLower(strings.SplitN(strings.ReplaceAll(r, "_", "-"), "-", 2)[0])
		if _, ok := supported[primary]; ok {
			return primary
		}
	}
	return ""
}
