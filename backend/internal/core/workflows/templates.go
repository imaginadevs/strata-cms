package workflows

import (
	"fmt"
	"regexp"
	"strings"
)

var itemTpl = regexp.MustCompile(`\{\{\s*item\.([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}`)

func renderTemplate(s string, item map[string]any) string {
	if s == "" || item == nil {
		return s
	}
	return itemTpl.ReplaceAllStringFunc(s, func(m string) string {
		sub := itemTpl.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		v, ok := item[sub[1]]
		if !ok || v == nil {
			return ""
		}
		return fmt.Sprint(v)
	})
}

func asStringSlice(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			out = append(out, fmt.Sprint(x))
		}
		return out
	default:
		s := strings.TrimSpace(fmt.Sprint(t))
		if s == "" {
			return nil
		}
		return []string{s}
	}
}

func mapString(cfg map[string]any, key string) string {
	if cfg == nil {
		return ""
	}
	v, ok := cfg[key]
	if !ok || v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func nestedMap(cfg map[string]any, key string) map[string]any {
	if cfg == nil {
		return nil
	}
	v, ok := cfg[key]
	if !ok || v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}
