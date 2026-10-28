package web

import (
	"context"
	"fmt"
	"io"

	"github.com/a-h/templ"
)

// Icon is a Lucide icon, inline, in the text colour. It is decorative: say
// what it means in text next to it or in an aria-label. A name missing from
// icons fails the render, so a typo shows up in tests, not as a blank.
func Icon(name string) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		path, ok := icons[name]
		if !ok {
			return fmt.Errorf("web: no icon %q in icons.go", name)
		}
		_, err := io.WriteString(w, `<svg class="icon" aria-hidden="true" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">`+path+`</svg>`)
		return err
	})
}

// icons are Lucide icons v0.460.0: the inner markup of each 24×24 SVG,
// rendered by Icon. Add one by copying its markup from
// https://lucide.dev/icons.
//
// Lucide is ISC licensed (https://lucide.dev/license): Copyright (c) for
// portions of Lucide are held by Cole Bemis 2013-2022 as part of Feather
// (MIT). All other copyright (c) for Lucide are held by Lucide Contributors
// 2022. Permission to use, copy, modify, and/or distribute this software for
// any purpose with or without fee is hereby granted, provided that the above
// copyright notice and this permission notice appear in all copies.
var icons = map[string]string{
	"activity":        `<path d="M22 12h-2.48a2 2 0 0 0-1.93 1.46l-2.35 8.36a.25.25 0 0 1-.48 0L9.24 2.18a.25.25 0 0 0-.48 0l-2.35 8.36A2 2 0 0 1 4.49 12H2"/>`,
	"book-open":       `<path d="M12 7v14"/><path d="M3 18a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h5a4 4 0 0 1 4 4 4 4 0 0 1 4-4h5a1 1 0 0 1 1 1v13a1 1 0 0 1-1 1h-6a3 3 0 0 0-3 3 3 3 0 0 0-3-3z"/>`,
	"calendar":        `<path d="M8 2v4"/><path d="M16 2v4"/><rect width="18" height="18" x="3" y="4" rx="2"/><path d="M3 10h18"/>`,
	"chevron-down":    `<path d="m6 9 6 6 6-6"/>`,
	"package":         `<path d="M11 21.73a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73z"/><path d="M12 22V12"/><path d="m3.3 7 7.703 4.734a2 2 0 0 0 1.994 0L20.7 7"/><path d="m7.5 4.27 9 5.15"/>`,
	"shopping-basket": `<path d="m15 11-1 9"/><path d="m19 11-4-7"/><path d="M2 11h20"/><path d="m3.5 11 1.6 7.4a2 2 0 0 0 2 1.6h9.8a2 2 0 0 0 2-1.6l1.7-7.4"/><path d="M4.5 15.5h15"/><path d="m5 11 4-7"/><path d="m9 11 1 9"/>`,
	"plus":            `<path d="M5 12h14"/><path d="M12 5v14"/>`,
	"user":            `<path d="M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/>`,
}
