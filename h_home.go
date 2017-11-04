package main

import (
	"log"
	"fmt"
	"net/http"
	"github.com/julienschmidt/httprouter"
)

const (
	HomeURL = "/"
)

func HomeIndex(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	log.Printf("INFO  GET /")

	db := OpenDB(true)
	defer CloseDB()

	ctx := AddressContext(w, r, "home_title", false, db)
	if !ctx.LoggedIn {
		return	// already redirected to /login if not logged in
	}

	if !ctx.CurrentAddress.Admin {
		redirect := fmt.Sprintf("/address/%d", ctx.CurrentAddress.ID)
		http.Redirect(w, r, redirect, http.StatusFound)
		return
	}

	addresses := []Address{}
	if err := db.Find(&addresses).Error; err != nil {
		log.Printf("ERROR HomeIndex:Addresses: %s", err)
	} else {
		//for _, address := range addresses {
		//	TODO get aliases
		//}
	}
	ctx.Addresses = addresses

	domains := []Domain{}
	if err := db.Find(&domains).Error; err != nil {
		log.Printf("ERROR HomeIndex:Domains: %s", err)
	} else {
		for _, domain := range domains {
			empty := true
			for _, address := range addresses {
				if address.DomainID == domain.ID {
					empty = false
					break
				}
			}
			if empty {
				ctx.Domains = append(ctx.Domains, domain)
			}
		}
	}

	RenderHtml(w, r, "home", ctx)
}
