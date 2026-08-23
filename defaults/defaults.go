// Package defaults imports the opinionated built-in adapters so they register.
// A fork that wants a different compiled-in set copies this file and swaps imports.
package defaults

import (
	_ "github.com/srhopkins/iugum/adapter/hook/hookbus"
	_ "github.com/srhopkins/iugum/adapter/memory/sqlitemem"
	_ "github.com/srhopkins/iugum/adapter/schedule/cronadapt"
	_ "github.com/srhopkins/iugum/adapter/watch/fswatch"
	_ "github.com/srhopkins/iugum/adapter/observe/execadapt"
	_ "github.com/srhopkins/iugum/adapter/observe/memadapt"
	_ "github.com/srhopkins/iugum/adapter/tracker/beadsadapt"
	_ "github.com/srhopkins/iugum/adapter/tracker/execadapt"
	_ "github.com/srhopkins/iugum/adapter/wiki/execadapt"
	_ "github.com/srhopkins/iugum/adapter/wiki/sbadapt"
)
