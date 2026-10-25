// PROTOTYPE (design system): throwaway, lives only on prototype/design-system.

package web

// protoIcons are Lucide icons v0.460.0 (ISC), the inner markup of each SVG.
var protoIcons = map[string]string{
	"activity": `<path d="M22 12h-2.48a2 2 0 0 0-1.93 1.46l-2.35 8.36a.25.25 0 0 1-.48 0L9.24 2.18a.25.25 0 0 0-.48 0l-2.35 8.36A2 2 0 0 1 4.49 12H2" />`,
	"arrow-right-left": `<path d="m16 3 4 4-4 4" /> <path d="M20 7H4" /> <path d="m8 21-4-4 4-4" /> <path d="M4 17h16" />`,
	"book-open": `<path d="M12 7v14" /> <path d="M3 18a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h5a4 4 0 0 1 4 4 4 4 0 0 1 4-4h5a1 1 0 0 1 1 1v13a1 1 0 0 1-1 1h-6a3 3 0 0 0-3 3 3 3 0 0 0-3-3z" />`,
	"calendar-days": `<path d="M8 2v4" /> <path d="M16 2v4" /> <rect width="18" height="18" x="3" y="4" rx="2" /> <path d="M3 10h18" /> <path d="M8 14h.01" /> <path d="M12 14h.01" /> <path d="M16 14h.01" /> <path d="M8 18h.01" /> <path d="M12 18h.01" /> <path d="M16 18h.01" />`,
	"chevron-right": `<path d="m9 18 6-6-6-6" />`,
	"clock-alert": `<path d="M12 6v6l4 2" /> <path d="M16 21.16a10 10 0 1 1 5-13.516" /> <path d="M20 11.5v6" /> <path d="M20 21.5h.01" />`,
	"ellipsis": `<circle cx="12" cy="12" r="1" /> <circle cx="19" cy="12" r="1" /> <circle cx="5" cy="12" r="1" />`,
	"package": `<path d="M11 21.73a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73z" /> <path d="M12 22V12" /> <path d="m3.3 7 7.703 4.734a2 2 0 0 0 1.994 0L20.7 7" /> <path d="m7.5 4.27 9 5.15" />`,
	"pencil": `<path d="M21.174 6.812a1 1 0 0 0-3.986-3.987L3.842 16.174a2 2 0 0 0-.5.83l-1.321 4.352a.5.5 0 0 0 .623.622l4.353-1.32a2 2 0 0 0 .83-.497z" /> <path d="m15 5 4 4" />`,
	"plus": `<path d="M5 12h14" /> <path d="M12 5v14" />`,
	"refrigerator": `<path d="M5 6a4 4 0 0 1 4-4h6a4 4 0 0 1 4 4v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6Z" /> <path d="M5 10h14" /> <path d="M15 7v6" />`,
	"search": `<circle cx="11" cy="11" r="8" /> <path d="m21 21-4.3-4.3" />`,
	"shopping-cart": `<circle cx="8" cy="21" r="1" /> <circle cx="19" cy="21" r="1" /> <path d="M2.05 2.05h2l2.66 12.42a2 2 0 0 0 2 1.58h9.78a2 2 0 0 0 1.95-1.57l1.65-7.43H5.12" />`,
	"snowflake": `<line x1="2" x2="22" y1="12" y2="12" /> <line x1="12" x2="12" y1="2" y2="22" /> <path d="m20 16-4-4 4-4" /> <path d="m4 8 4 4-4 4" /> <path d="m16 4-4 4-4-4" /> <path d="m8 20 4-4 4 4" />`,
	"trash-2": `<path d="M3 6h18" /> <path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6" /> <path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2" /> <line x1="10" x2="10" y1="11" y2="17" /> <line x1="14" x2="14" y1="11" y2="17" />`,
	"user": `<path d="M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2" /> <circle cx="12" cy="7" r="4" />`,
	"x": `<path d="M18 6 6 18" /> <path d="m6 6 12 12" />`,
}
