package main

import (
	"log"
	"fmt"
	"net/http"
	"github.com/julienschmidt/httprouter"
	//"github.com/nicksnyder/go-i18n/i18n"
)

func HelpShow(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	//t, _ := i18n.Tfunc(Language)
	title := ps.ByName("page")
	page := fmt.Sprintf("help_%s_%s", Language, title)
	log.Printf("INFO  GET /help/%s", page)
	prefix := "../"

	ctx := AddressContext(w, r, title, false, prefix, nil)

	RenderHtml(w, r, page, ctx)
}
