//go:build !windows

package shellmenu

import (
	"errors"

	"github.com/rightconvert/rightconvert/internal/rules"
)

var errUnsupported = errors.New("the context menu is only available on Windows")

func Register(*rules.Set, Options) (int, error) { return 0, errUnsupported }
func Unregister() error                         { return nil }
func Registered() ([]string, error)             { return nil, nil }
func NotifyShell()                              {}
