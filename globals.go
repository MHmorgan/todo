package main

var ignoreDirs = []string{
	".*",
	"build",
	"venv",
	"__pycache__",
}

var ignoreFiles = map[string]any{
	".pyc":      nil,
	".o":        nil,
	".a":        nil,
	".so":       nil,
	".mp3":      nil,
	".zip":      nil,
	".gz":       nil,
	".png":      nil,
	".jpg":      nil,
	".gif":      nil,
	".jpeg":     nil,
	".bmp":      nil,
	".ico":      nil,
	".pdf":      nil,
	".DS_Store": nil,
	".epub":     nil,
	".mobi":     nil,
	".ttf":      nil,
	".otf":      nil,
	".plist":    nil,
}

var patterns = map[string]string{
	"alpha":  `^\s*(?://|#|/?\*|--)\s+(@[A-Z]+)\s(.*)`,
	"todo":   `^\s*(?://|#|/?\*|--)\s+(TODO):?\s?(\S.*)`,
	"common": `^\s*(?://|#|/?\*|--)\s+(TODO|FIXME|XXX|BUG|HACK|NOTE|REVIEW|NB|IDEA|QUESTION|COMBAK|TEMP|DEBUG|OPTIMIZE|WARNING|ERROR|DEPRECATED|SECURITY):?\s?(\S.*)`,
}
