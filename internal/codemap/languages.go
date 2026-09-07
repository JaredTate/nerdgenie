package codemap

import (
	"regexp"
	"strings"
)

// The definition patterns, one small set per language family. Each finds a
// line that defines a name and says where the name and its parameters sit.
// The script patterns allow indentation in front, because a browser game is
// written inside a function that runs at once as often as it is a module,
// and a function inside it is still a function the map lists.

var (
	scriptFunction = regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s*\*?\s*([A-Za-z_$][\w$]*)\s*\(([^)]*)\)`)
	scriptArrow    = regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*(?::[^=]+)?=\s*(?:async\s*)?(?:\(([^)]*)\)|[A-Za-z_$][\w$]*)\s*=>`)
	scriptClass    = regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:abstract\s+)?class\s+([A-Za-z_$][\w$]*)`)
	scriptConst    = regexp.MustCompile(`^export\s+const\s+([A-Za-z_$][\w$]*)\s*(?::[^=]+)?=`)
	// scriptObject is a named object literal opening on its own line, such
	// as "const view = {", whose method-shaped lines are its methods.
	scriptObject = regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*(?::[^=]+)?=\s*\{\s*$`)
	// scriptKeyObject is a nested object literal opening under a key, such as
	// "handlers: {", and scriptDefaultObject is "export default {".
	scriptKeyObject     = regexp.MustCompile(`^\s+[A-Za-z_$][\w$]*\s*:\s*\{\s*,?$`)
	scriptDefaultObject = regexp.MustCompile(`^export\s+default\s+\{\s*$`)
	scriptMethod        = regexp.MustCompile(`^\s+(?:public\s+|private\s+|protected\s+|static\s+|async\s+|readonly\s+)*(?:get\s+|set\s+)?([A-Za-z_$][\w$]*)\s*\(([^)]*)\)\s*(?::\s*[^{]+)?\{`)
	// scriptPropertyFunction and scriptPropertyArrow are the two other shapes
	// an object literal's method takes: "clear: function () {" and
	// "mark: (at) =>".
	scriptPropertyFunction = regexp.MustCompile(`^\s+([A-Za-z_$][\w$]*)\s*:\s*(?:async\s+)?function\s*\*?\s*\(([^)]*)\)`)
	scriptPropertyArrow    = regexp.MustCompile(`^\s+([A-Za-z_$][\w$]*)\s*:\s*(?:async\s*)?\(([^)]*)\)\s*=>`)
	scriptTest             = regexp.MustCompile(`(?:^|[^\w.])(?:test|it)\s*\(`)

	goFunc   = regexp.MustCompile(`^func\s+([A-Za-z_]\w*)\s*\(([^)]*)\)`)
	goMethod = regexp.MustCompile(`^func\s+\([^)]*\)\s*([A-Za-z_]\w*)\s*\(([^)]*)\)`)
	goType   = regexp.MustCompile(`^type\s+([A-Za-z_]\w*)\s+`)
	goTest   = regexp.MustCompile(`^func (?:Test|Fuzz|Benchmark)\w*\(`)

	pythonDef   = regexp.MustCompile(`^def\s+([A-Za-z_]\w*)\s*\(([^)]*)\)`)
	pythonMeth  = regexp.MustCompile(`^\s+def\s+([A-Za-z_]\w*)\s*\(([^)]*)\)`)
	pythonClass = regexp.MustCompile(`^class\s+([A-Za-z_]\w*)`)
	pythonTest  = regexp.MustCompile(`^\s*def\s+test_\w*\(`)

	rustFn   = regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?(?:async\s+|unsafe\s+|const\s+)*fn\s+([A-Za-z_]\w*)\s*(?:<[^>]*>)?\s*\(([^)]*)\)`)
	rustType = regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?(?:struct|enum|trait|type)\s+([A-Za-z_]\w*)`)
	rustTest = regexp.MustCompile(`^\s*#\[(?:tokio::)?test\]`)

	rubyDef   = regexp.MustCompile(`^\s*def\s+(?:self\.)?([A-Za-z_]\w*[?!=]?)\s*(?:\(([^)]*)\))?`)
	rubyClass = regexp.MustCompile(`^\s*(?:class|module)\s+([A-Z]\w*)`)
	rubyTest  = regexp.MustCompile(`^\s*(?:it|test|specify)\s+['"]`)

	// cFunction is a function definition with a body on the same line or the
	// next: a return type, a name, parameters, then a brace or nothing.
	cFunction = regexp.MustCompile(`^(?:[A-Za-z_][\w:<>,\s\*&]*\s+[\*&]*)([A-Za-z_]\w*)\s*\(([^;{}]*)\)\s*(?:const\s*)?(?:override\s*)?(?:noexcept\s*)?(?:\{|$)`)
	cMethod   = regexp.MustCompile(`^\s+(?:[A-Za-z_][\w:<>,\s\*&]*\s+[\*&]*)?([A-Za-z_]\w*)\s*\(([^;{}]*)\)\s*(?:const\s*)?(?:override\s*)?(?:noexcept\s*)?\{`)
	cClass    = regexp.MustCompile(`^\s*(?:class|struct)\s+([A-Za-z_]\w*)\s*(?::|\{|$)`)
	cTest     = regexp.MustCompile(`^\s*(?:TEST|TEST_F|TEST_P|BOOST_AUTO_TEST_CASE|TEST_CASE)\s*\(`)

	javaMethod = regexp.MustCompile(`^\s+(?:public|private|protected|static|final|abstract|synchronized|\s)*[\w<>\[\],\s]+\s+([A-Za-z_]\w*)\s*\(([^)]*)\)\s*(?:throws[^{]*)?\{`)
	javaClass  = regexp.MustCompile(`^\s*(?:public\s+|private\s+|protected\s+|abstract\s+|final\s+|static\s+)*(?:class|interface|enum|record)\s+([A-Za-z_]\w*)`)
	javaTest   = regexp.MustCompile(`^\s*@Test\b`)
)

// slashComment reads a // or /// or /** ... */ line.
func slashComment(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(trimmed, "///"):
		return strings.TrimSpace(trimmed[3:]), true
	case strings.HasPrefix(trimmed, "//"):
		return strings.TrimSpace(trimmed[2:]), true
	case strings.HasPrefix(trimmed, "/**") || strings.HasPrefix(trimmed, "/*"):
		inner := strings.TrimLeft(trimmed, "/*")
		return strings.TrimSpace(strings.TrimSuffix(inner, "*/")), true
	case strings.HasPrefix(trimmed, "*/"):
		return "", true
	case strings.HasPrefix(trimmed, "*"):
		return strings.TrimSpace(strings.TrimSuffix(trimmed[1:], "*/")), true
	}
	return "", false
}

// hashComment reads a # line.
func hashComment(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "#!") && !strings.HasPrefix(trimmed, "#[") {
		return strings.TrimSpace(strings.TrimLeft(trimmed, "#")), true
	}
	return "", false
}

// pythonComment reads a # line and a decorator line, which sits between a
// comment and its def.
func pythonComment(line string) (string, bool) {
	if text, ok := hashComment(line); ok {
		return text, true
	}
	if strings.HasPrefix(strings.TrimSpace(line), "@") {
		return "", true
	}
	return "", false
}

// rustComment reads a /// or // line and an attribute line above a fn.
func rustComment(line string) (string, bool) {
	if text, ok := slashComment(line); ok {
		return text, true
	}
	if strings.HasPrefix(strings.TrimSpace(line), "#[") {
		return "", true
	}
	return "", false
}

var languages = map[string]language{
	".js": scriptLanguage, ".mjs": scriptLanguage, ".cjs": scriptLanguage, ".jsx": scriptLanguage,
	".ts": scriptLanguage, ".tsx": scriptLanguage, ".mts": scriptLanguage,
	".go": {defs: []definer{{goMethod, "method", 1, 2, false}, {goFunc, "func", 1, 2, false}, {goType, "type", 1, 0, false}}, comment: slashComment, test: goTest, braces: true},
	".py": {defs: []definer{{pythonDef, "func", 1, 2, false}, {pythonMeth, "method", 1, 2, true}, {pythonClass, "class", 1, 0, false}}, openers: []*regexp.Regexp{pythonClass}, comment: pythonComment, test: pythonTest, docstringBelow: true},
	".rs": {defs: []definer{{rustFn, "func", 1, 2, false}, {rustType, "type", 1, 0, false}}, comment: rustComment, test: rustTest, braces: true},
	".rb": {defs: []definer{{rubyClass, "class", 1, 0, false}, {rubyDef, "func", 1, 2, false}}, openers: []*regexp.Regexp{rubyClass}, comment: hashComment, test: rubyTest},
	".c":  cLanguage, ".h": cLanguage, ".cc": cLanguage, ".cpp": cLanguage, ".cxx": cLanguage, ".hpp": cLanguage, ".hh": cLanguage,
	".java": {defs: []definer{{javaClass, "class", 1, 0, false}, {javaMethod, "method", 1, 2, true}}, openers: []*regexp.Regexp{javaClass}, comment: slashComment, test: javaTest, braces: true},
}

var scriptLanguage = language{
	defs: []definer{
		{scriptFunction, "func", 1, 2, false},
		{scriptArrow, "func", 1, 2, false},
		{scriptClass, "class", 1, 0, false},
		{scriptConst, "const", 1, 0, false},
		{scriptObject, "const", 1, 0, false},
		{scriptMethod, "method", 1, 2, true},
		{scriptPropertyFunction, "method", 1, 2, true},
		{scriptPropertyArrow, "method", 1, 2, true},
	},
	openers: []*regexp.Regexp{scriptClass, scriptObject, scriptKeyObject, scriptDefaultObject},
	braces:  true,
	comment: slashComment,
	test:    scriptTest,
}

var cLanguage = language{
	defs: []definer{
		{cClass, "class", 1, 0, false},
		{cMethod, "method", 1, 2, true},
		{cFunction, "func", 1, 2, false},
	},
	openers: []*regexp.Regexp{cClass},
	braces:  true,
	comment: slashComment,
	test:    cTest,
}

// languageOf finds the reader for a file extension.
func languageOf(extension string) (language, bool) {
	spoken, known := languages[strings.ToLower(extension)]
	return spoken, known
}
