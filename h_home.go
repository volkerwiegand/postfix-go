package main

import (
	"log"
	"net/http"
	"github.com/julienschmidt/httprouter"
	"github.com/nicksnyder/go-i18n/i18n"
)

func HomeURL() string {
	return Base_URL
}

func HomeIndex(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	t, _ := i18n.Tfunc(Language)
	log.Printf("INFO  GET %s", HomeURL())

	db := OpenDB(true)
	defer CloseDB()

	ctx := AddressContext(w, r, "home_title", false, db)
	if !ctx.LoggedIn {
		return	// already redirected to /login if not logged in
	}

	if ctx.CurrentAddress.Admin == false {
		SetFlash(w, F_INFO, t("flash_login_update"))
		http.Redirect(w, r, PasswordURL(), http.StatusFound)
		return
	}

	addresses := []Address{}
	if err := db.Find(&addresses).Error; err != nil {
		log.Printf("ERROR HomeIndex:Addresses: %s", err)
	} else {
		for index, _ := range addresses {
			address := &addresses[index]
			address.AddressSetup(db)
		}
	}
	ctx.Addresses = addresses

	domains := []Domain{}
	if err := db.Find(&domains).Error; err != nil {
		log.Printf("ERROR HomeIndex:Domains: %s", err)
	} else {
		for _, domain := range domains {
			domain.DomainSetup(db)
			if len(domain.Addresses) == 0 {
				ctx.Domains = append(ctx.Domains, domain)
			}
		}
	}

	RenderHtml(w, r, "home", ctx)
}
