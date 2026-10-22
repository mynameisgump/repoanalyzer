package loc

import (
	"path/filepath"
	"strings"
)

var byName = map[string]string{
	"Makefile": "Makefile", "GNUmakefile": "Makefile",
	"Dockerfile": "Dockerfile", "Containerfile": "Dockerfile",
	"CMakeLists.txt": "CMake", "Jenkinsfile": "Groovy",
	"Rakefile": "Ruby", "Gemfile": "Ruby", "BUILD": "Starlark", "BUILD.bazel": "Starlark", "WORKSPACE": "Starlark",
}

var byExt = map[string]string{
	".go": "Go", ".rs": "Rust", ".py": "Python", ".pyi": "Python", ".rb": "Ruby",
	".js": "JavaScript", ".mjs": "JavaScript", ".cjs": "JavaScript", ".jsx": "JavaScript",
	".ts": "TypeScript", ".mts": "TypeScript", ".cts": "TypeScript", ".tsx": "TypeScript",
	".java": "Java", ".kt": "Kotlin", ".kts": "Kotlin", ".scala": "Scala", ".groovy": "Groovy", ".gradle": "Groovy",
	".c": "C", ".h": "C", ".cc": "C++", ".cpp": "C++", ".cxx": "C++", ".hpp": "C++", ".hh": "C++", ".hxx": "C++",
	".m": "Objective-C", ".mm": "Objective-C", ".swift": "Swift", ".cs": "C#", ".fs": "F#", ".vb": "Visual Basic",
	".php": "PHP", ".pl": "Perl", ".pm": "Perl", ".lua": "Lua", ".r": "R", ".jl": "Julia", ".dart": "Dart",
	".ex": "Elixir", ".exs": "Elixir", ".erl": "Erlang", ".hs": "Haskell", ".ml": "OCaml", ".clj": "Clojure",
	".zig": "Zig", ".nim": "Nim", ".v": "Verilog", ".sv": "SystemVerilog", ".vhd": "VHDL",
	".gd": "GDScript", ".tscn": "Godot Scene", ".tres": "Godot Resource", ".shader": "Shader", ".gdshader": "Shader",
	".glsl": "GLSL", ".hlsl": "HLSL", ".wgsl": "WGSL",
	".sh": "Shell", ".bash": "Shell", ".zsh": "Shell", ".fish": "Shell", ".ps1": "PowerShell", ".bat": "Batch",
	".sql": "SQL", ".graphql": "GraphQL", ".gql": "GraphQL", ".proto": "Protobuf",
	".html": "HTML", ".htm": "HTML", ".css": "CSS", ".scss": "SCSS", ".sass": "SCSS", ".less": "Less",
	".vue": "Vue", ".svelte": "Svelte", ".astro": "Astro",
	".json": "JSON", ".jsonc": "JSON", ".yaml": "YAML", ".yml": "YAML", ".toml": "TOML", ".xml": "XML",
	".ini": "INI", ".cfg": "INI", ".tf": "Terraform", ".hcl": "HCL", ".nix": "Nix", ".cmake": "CMake",
	".md": "Markdown", ".mdx": "Markdown", ".rst": "reStructuredText", ".tex": "TeX", ".txt": "Text",
	".csv": "CSV", ".svg": "SVG",
}

// Detect guesses a file's language from its name or extension.
func Detect(path string) string {
	base := filepath.Base(path)
	if l, ok := byName[base]; ok {
		return l
	}
	if l, ok := byExt[strings.ToLower(filepath.Ext(base))]; ok {
		return l
	}
	return "Other"
}

// Languages that hold data, config or prose rather than program logic.
var nonCode = map[string]bool{
	"JSON": true, "YAML": true, "TOML": true, "XML": true, "INI": true, "CSV": true, "SVG": true,
	"Markdown": true, "reStructuredText": true, "TeX": true, "Text": true, "Other": true,
}

// IsCode reports whether a language from Detect is a programming language,
// as opposed to data, config or docs (which test fixtures are often made of).
func IsCode(lang string) bool {
	return !nonCode[lang]
}
