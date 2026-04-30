package shellscripts

import _ "embed"

//go:embed undo.zsh
var Zsh []byte

//go:embed undo.bash
var Bash []byte

//go:embed undo.fish
var Fish []byte
