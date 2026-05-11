package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"dr600ab-api/internal/model"
)

//go:embed locales/*/*.json
var localeFS embed.FS

type Translator struct {
	defaultLocale string
	resources     map[string]map[string]map[string]string
}

func New(defaultLocale string) (*Translator, error) {
	t := &Translator{
		defaultLocale: strings.TrimSpace(defaultLocale),
		resources:     make(map[string]map[string]map[string]string),
	}
	if t.defaultLocale == "" {
		t.defaultLocale = "zh-CN"
	}

	files, err := fs.Glob(localeFS, "locales/*/*.json")
	if err != nil {
		return nil, fmt.Errorf("扫描语言包失败: %w", err)
	}
	for _, name := range files {
		parts := strings.Split(name, "/")
		if len(parts) != 3 {
			continue
		}
		locale := parts[1]
		namespace := strings.TrimSuffix(parts[2], ".json")

		data, err := localeFS.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("读取语言包 %s 失败: %w", name, err)
		}
		values := make(map[string]string)
		if err := json.Unmarshal(data, &values); err != nil {
			return nil, fmt.Errorf("解析语言包 %s 失败: %w", name, err)
		}

		if _, ok := t.resources[locale]; !ok {
			t.resources[locale] = make(map[string]map[string]string)
		}
		t.resources[locale][namespace] = values
	}

	if _, ok := t.resources[t.defaultLocale]; !ok {
		return nil, fmt.Errorf("默认语言包 %s 不存在", t.defaultLocale)
	}
	return t, nil
}

func (t *Translator) Meta() model.LocaleMeta {
	return model.LocaleMeta{
		Default:    t.defaultLocale,
		Supported:  t.SupportedLocales(),
		Namespaces: t.Namespaces(),
	}
}

func (t *Translator) SupportedLocales() []string {
	locales := make([]string, 0, len(t.resources))
	for locale := range t.resources {
		locales = append(locales, locale)
	}
	sort.Strings(locales)
	return locales
}

func (t *Translator) Namespaces() []string {
	seen := make(map[string]struct{})
	for _, namespaces := range t.resources {
		for namespace := range namespaces {
			seen[namespace] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for namespace := range seen {
		result = append(result, namespace)
	}
	sort.Strings(result)
	return result
}

func (t *Translator) Normalize(locale string) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return t.defaultLocale
	}
	locale = strings.ReplaceAll(locale, "_", "-")
	for supported := range t.resources {
		if strings.EqualFold(supported, locale) {
			return supported
		}
	}

	low := strings.ToLower(locale)
	for supported := range t.resources {
		supportedLow := strings.ToLower(supported)
		if strings.HasPrefix(low, strings.SplitN(supportedLow, "-", 2)[0]) {
			return supported
		}
	}
	return t.defaultLocale
}

func (t *Translator) T(locale, namespace, key string) string {
	locale = t.Normalize(locale)
	if value, ok := t.lookup(locale, namespace, key); ok {
		return value
	}
	if locale != t.defaultLocale {
		if value, ok := t.lookup(t.defaultLocale, namespace, key); ok {
			return value
		}
	}
	return key
}

func (t *Translator) lookup(locale, namespace, key string) (string, bool) {
	namespaces, ok := t.resources[locale]
	if !ok {
		return "", false
	}
	values, ok := namespaces[namespace]
	if !ok {
		return "", false
	}
	value, ok := values[key]
	return value, ok
}
