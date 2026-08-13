package tui

import "strings"

type themeEntry struct {
	Name    string
	Label   string
	Palette palette
	IsDark  bool
}

var darkPalette = palette{
	panel:    "#0e0e10",
	promptBg: "#262626",
	line:     "#242429",
	line2:    "#414147",
	ink:      "#ececee",
	muted:    "#9a9aa2",
	faint:    "#8a8a92",
	faintest: "#7c7c82",
	accent:   "#caff3f",
	green:    "#5dd1a4",
	red:      "#ff7a7a",
	amber:    "#ffc25c",
	blue:     "#7db4ff",
	onAccent: "#000000",
	cardRun:  "#5a6b2e",
	cardErr:  "#6b3434",
}

var draculaPalette = palette{
	panel:    "#282a36",
	promptBg: "#383c4d",
	line:     "#363a4b",
	line2:    "#484c62",
	ink:      "#f8f8f2",
	muted:    "#b9bccb",
	faint:    "#a2a5b8",
	faintest: "#9195ac",
	accent:   "#bd93f9",
	green:    "#50fa7b",
	red:      "#ff5555",
	amber:    "#ffb86c",
	blue:     "#8be9fd",
	onAccent: "#000000",
	cardRun:  "#7c6aa6",
	cardErr:  "#98505a",
}

var nordPalette = palette{
	panel:    "#3b4252",
	promptBg: "#464f62",
	line:     "#434c5e",
	line2:    "#4c566a",
	ink:      "#eceff4",
	muted:    "#c8cfda",
	faint:    "#b4bdcb",
	faintest: "#a5afc1",
	accent:   "#88c0d0",
	green:    "#a3be8c",
	red:      "#bf616a",
	amber:    "#d08770",
	blue:     "#81a1c1",
	onAccent: "#000000",
	cardRun:  "#4c6672",
	cardErr:  "#6b4a51",
}

var gruvboxPalette = palette{
	panel:    "#32302f",
	promptBg: "#3c3836",
	line:     "#504945",
	line2:    "#665c54",
	ink:      "#ebdbb2",
	muted:    "#c9b99a",
	faint:    "#b7a78d",
	faintest: "#a89984",
	accent:   "#8ec07c",
	green:    "#b8bb26",
	red:      "#fb4934",
	amber:    "#fabd2f",
	blue:     "#83a598",
	onAccent: "#000000",
	cardRun:  "#6f8460",
	cardErr:  "#a5493d",
}

var tokyoNightPalette = palette{
	panel:    "#1e2030",
	promptBg: "#2c3149",
	line:     "#262a3d",
	line2:    "#3b4261",
	ink:      "#c8d3f5",
	muted:    "#a9b1d0",
	faint:    "#9099b2",
	faintest: "#838ba8",
	accent:   "#82aaff",
	green:    "#c3e88d",
	red:      "#ff757f",
	amber:    "#ffc777",
	blue:     "#86e1fc",
	onAccent: "#000000",
	cardRun:  "#4b5d8b",
	cardErr:  "#7d4857",
}

var catppuccinPalette = palette{
	panel:    "#1e1e2e",
	promptBg: "#34364b",
	line:     "#313244",
	line2:    "#45475a",
	ink:      "#cdd6f4",
	muted:    "#a6adc8",
	faint:    "#9399b2",
	faintest: "#83889f",
	accent:   "#cba6f7",
	green:    "#a6e3a1",
	red:      "#f38ba8",
	amber:    "#f9e2af",
	blue:     "#89b4fa",
	onAccent: "#000000",
	cardRun:  "#7a6a99",
	cardErr:  "#8a5b72",
}

var oneDarkPalette = palette{
	panel:    "#2e323b",
	promptBg: "#3a3f4a",
	line:     "#393f4a",
	line2:    "#4b525f",
	ink:      "#abb2bf",
	muted:    "#a2a9b6",
	faint:    "#9aa1af",
	faintest: "#969cab",
	accent:   "#61afef",
	green:    "#98c379",
	red:      "#e06c75",
	amber:    "#e5c07b",
	blue:     "#56b6c2",
	onAccent: "#000000",
	cardRun:  "#496c8c",
	cardErr:  "#7c515b",
}

var solarizedDarkPalette = palette{
	panel:    "#073642",
	promptBg: "#0b3b46",
	line:     "#123f48",
	line2:    "#4b636c",
	ink:      "#cdd6d6",
	muted:    "#a9b3b3",
	faint:    "#9ba5a5",
	faintest: "#929c9c",
	accent:   "#3bb3a6",
	green:    "#859900",
	red:      "#dc322f",
	amber:    "#b58900",
	blue:     "#268bd2",
	onAccent: "#000000",
	cardRun:  "#1d6b6c",
	cardErr:  "#6d393d",
}

var rosePinePalette = palette{
	panel:    "#1f1d2e",
	promptBg: "#2f2b47",
	line:     "#2b2840",
	line2:    "#403d52",
	ink:      "#e0def4",
	muted:    "#a8a3c0",
	faint:    "#928ea9",
	faintest: "#8985a0",
	accent:   "#ebbcba",
	green:    "#31748f",
	red:      "#eb6f92",
	amber:    "#f6c177",
	blue:     "#9ccfd8",
	onAccent: "#000000",
	cardRun:  "#7c6673",
	cardErr:  "#7c4662",
}

var everforestPalette = palette{
	panel:    "#333c43",
	promptBg: "#3d484d",
	line:     "#414b52",
	line2:    "#55636b",
	ink:      "#d3c6aa",
	muted:    "#b0bab0",
	faint:    "#a4aea3",
	faintest: "#9ca99b",
	accent:   "#a7c080",
	green:    "#83c092",
	red:      "#e67e80",
	amber:    "#dbbc7f",
	blue:     "#7fbbb3",
	onAccent: "#000000",
	cardRun:  "#798b6b",
	cardErr:  "#9c676b",
}

var neonPalette = palette{
	panel:    "#050b06",
	promptBg: "#0c180d",
	line:     "#1c3820",
	line2:    "#2c5230",
	ink:      "#c9ffd2",
	muted:    "#80db8f",
	faint:    "#6eca7d",
	faintest: "#74c468",
	accent:   "#00e5c8",
	green:    "#39ff6a",
	red:      "#ff4d6d",
	amber:    "#f4ff3a",
	blue:     "#22e0ff",
	onAccent: "#001410",
	cardRun:  "#1f8a6e",
	cardErr:  "#9a4042",
}

var lightPalette = palette{
	panel:    "#efebd4",
	promptBg: "#e3ddc2",
	line:     "#d8d2bd",
	line2:    "#b7b199",
	ink:      "#22201a",
	muted:    "#4b5149",
	faint:    "#575e55",
	faintest: "#636a61",
	accent:   "#54700a",
	green:    "#1e725c",
	red:      "#c02434",
	amber:    "#8a5f00",
	blue:     "#1f66c0",
	onAccent: "#ffffff",
	cardRun:  "#b0be7e",
	cardErr:  "#d8b0a8",
}

var solarizedLightPalette = palette{
	panel:    "#eee8d5",
	promptBg: "#e1d9be",
	line:     "#d8d1bc",
	line2:    "#c0b89e",
	ink:      "#304049",
	muted:    "#495b61",
	faint:    "#506469",
	faintest: "#576b72",
	accent:   "#0c665c",
	green:    "#859900",
	red:      "#dc322f",
	amber:    "#7a5c00",
	blue:     "#268bd2",
	onAccent: "#ffffff",
	cardRun:  "#7fbaaf",
	cardErr:  "#d8837a",
}

var dunePalette = palette{
	panel:    "#f2e9d8",
	promptBg: "#e9dcbf",
	line:     "#d9c7a3",
	line2:    "#c2a97c",
	ink:      "#2b241a",
	muted:    "#473e32",
	faint:    "#554a3a",
	faintest: "#655648",
	accent:   "#724028",
	green:    "#38572a",
	red:      "#872d24",
	amber:    "#6d4600",
	blue:     "#2f5680",
	onAccent: "#fdf6ea",
	cardRun:  "#b08a4a",
	cardErr:  "#b57560",
}

var themeRegistry = []themeEntry{
	{Name: "dark", Label: "Dark", Palette: darkPalette, IsDark: true},
	{Name: "dracula", Label: "Dracula", Palette: draculaPalette, IsDark: true},
	{Name: "nord", Label: "Nord", Palette: nordPalette, IsDark: true},
	{Name: "gruvbox", Label: "Gruvbox", Palette: gruvboxPalette, IsDark: true},
	{Name: "tokyo-night", Label: "Tokyo Night", Palette: tokyoNightPalette, IsDark: true},
	{Name: "catppuccin", Label: "Catppuccin", Palette: catppuccinPalette, IsDark: true},
	{Name: "one-dark", Label: "One Dark", Palette: oneDarkPalette, IsDark: true},
	{Name: "solarized-dark", Label: "Solarized Dark", Palette: solarizedDarkPalette, IsDark: true},
	{Name: "rose-pine", Label: "Rosé Pine", Palette: rosePinePalette, IsDark: true},
	{Name: "everforest", Label: "Everforest", Palette: everforestPalette, IsDark: true},
	{Name: "neon", Label: "Neon", Palette: neonPalette, IsDark: true},
	{Name: "light", Label: "Light", Palette: lightPalette, IsDark: false},
	{Name: "solarized-light", Label: "Solarized Light", Palette: solarizedLightPalette, IsDark: false},
	{Name: "dune", Label: "Dune", Palette: dunePalette, IsDark: false},
}

func lookupTheme(name string) (themeEntry, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, entry := range themeRegistry {
		if strings.ToLower(entry.Name) == name || strings.ToLower(entry.Label) == name {
			return entry, true
		}
	}
	return themeEntry{}, false
}
