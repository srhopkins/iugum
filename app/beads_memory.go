package app

import "github.com/srhopkins/iugum/adapter/tracker/beadsadapt"

func (a *App) bindBeadsMemory() {
	beadsadapt.UseMemory(a.Memory)
}
