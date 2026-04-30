package intercept

import _ "embed"

//go:embed csrc/intercept.c
var Source []byte
