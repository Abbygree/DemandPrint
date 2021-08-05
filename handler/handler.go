package handler

import (
	"DemandPrint/config"
	"github.com/Abbygree/gogmail"
)

type (
	Handler struct {
		Config *config.Config
		Pub    Publisher
		Gmail  *gogmail.GMail
	}

	Publisher interface {
	}
)
