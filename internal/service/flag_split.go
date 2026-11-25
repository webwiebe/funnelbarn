package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ParseSplitSpec turns "true=50,false=50" into the stored split JSON. It checks
// the shape only; ValidateSplit checks the percentages against the variants.
func ParseSplitSpec(spec string) (string, error) {
	split := map[string]int{}
	for _, part := range strings.Split(spec, ",") {
		name, pct, ok := strings.Cut(strings.TrimSpace(part), "=")
		n, err := strconv.Atoi(strings.TrimSpace(pct))
		if !ok || strings.TrimSpace(name) == "" || err != nil {
			return "", fmt.Errorf("split %q: want variant=percent pairs such as true=50,false=50", spec)
		}
		split[strings.TrimSpace(name)] = n
	}
	b, err := json.Marshal(split)
	return string(b), err
}

// ValidateSplit checks a split against the flag's variants: every name is a
// variant, every percentage is 0 to 100, and they add up to 100.
func ValidateSplit(variantsJSON, splitJSON string) error {
	var split map[string]int
	if err := json.Unmarshal([]byte(splitJSON), &split); err != nil {
		return fmt.Errorf("split: %w", err)
	}
	var variants map[string]any
	if err := json.Unmarshal([]byte(variantsJSON), &variants); err != nil {
		return fmt.Errorf("variants: %w", err)
	}
	total := 0
	names := make([]string, 0, len(split))
	for name := range split {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, ok := variants[name]; !ok {
			return fmt.Errorf("split names variant %q, which the flag does not have", name)
		}
		if split[name] < 0 || split[name] > 100 {
			return fmt.Errorf("split %q is %d, want 0 to 100", name, split[name])
		}
		total += split[name]
	}
	if total != 100 {
		return fmt.Errorf("split adds up to %d, want 100", total)
	}
	return nil
}
