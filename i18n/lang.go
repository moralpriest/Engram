package i18n

import (
	"sync"
)

var (
	currentLang string
	langMu      sync.RWMutex
)

const (
	LangEN = "en"
	LangFR = "fr"
	LangES = "es"
	LangDE = "de"
	LangRU = "ru"
	LangPT = "pt"
	LangZH = "zh"
	LangJA = "ja"
	LangEO = "eo"
	LangIT = "it"
	LangNL = "nl"
)

var availableLanguages = map[string]string{
	LangEN: "English",
	LangFR: "Français",
	LangES: "Español",
	LangDE: "Deutsch",
	LangRU: "Русский",
	LangPT: "Português",
	LangZH: "简体中文",
	LangJA: "日本語",
	LangEO: "Esperanto",
	LangIT: "Italiano",
	LangNL: "Nederlands",
}

func AvailableLanguages() map[string]string {
	return availableLanguages
}

func LanguageOrder() []string {
	return []string{LangEN, LangFR, LangES, LangDE, LangRU, LangPT, LangZH, LangJA, LangEO, LangIT, LangNL}
}

func T(key string) string {
	langMu.RLock()
	lang := currentLang
	langMu.RUnlock()

	if lang == "" {
		lang = LangEN
	}

	if lang == LangEN {
		if v, ok := stringsEN[key]; ok {
			return v
		}
		return key
	}

	var translations map[string]string
	switch lang {
	case LangFR:
		translations = stringsFR
	case LangES:
		translations = stringsES
	case LangDE:
		translations = stringsDE
	case LangRU:
		translations = stringsRU
	case LangPT:
		translations = stringsPT
	case LangZH:
		translations = stringsZH
	case LangJA:
		translations = stringsJA
	case LangEO:
		translations = stringsEO
	case LangIT:
		translations = stringsIT
	case LangNL:
		translations = stringsNL
	}

	if translations != nil {
		if v, ok := translations[key]; ok {
			return v
		}
	}

	if v, ok := stringsEN[key]; ok {
		return v
	}

	return key
}

func SetLanguage(code string) {
	langMu.Lock()
	currentLang = code
	langMu.Unlock()
}

func GetLanguage() string {
	langMu.RLock()
	defer langMu.RUnlock()
	return currentLang
}

func SetLanguageFromIndex(idx int) {
	order := LanguageOrder()
	if idx >= 0 && idx < len(order) {
		SetLanguage(order[idx])
	}
}

func GetLanguageIndex() int {
	langMu.RLock()
	lang := currentLang
	langMu.RUnlock()
	order := LanguageOrder()
	for i, l := range order {
		if l == lang {
			return i
		}
	}
	return 0
}
