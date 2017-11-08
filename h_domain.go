package main

import (
	"os"
	"log"
	"fmt"
	"time"
	"strings"
	"strconv"
	"net/http"
	"github.com/julienschmidt/httprouter"
	"github.com/jinzhu/gorm"
	"github.com/nicksnyder/go-i18n/i18n"
)

type Domain struct {
	ID            int         `gorm:"primary_key"`
	Name          string      `gorm:"unique_index"`
	CreatedAt     time.Time
	CreatedBy     int         `gorm:"index"`
	UpdatedAt     time.Time
	UpdatedBy     int         `gorm:"index"`
	// Computed values
	Addresses     []Address
	AddressCount  int         `sql:"-"`
	Selected      bool        `sql:"-"`
	ConfirmDelete string      `sql:"-"`
}

func DomainInit() {
	db := OpenDB(true)
	defer CloseDB()

	if err := db.AutoMigrate(&Domain{}).Error; err != nil {
		log.Printf("FATAL DomainInit:AutoMigrate: %s", err)
		os.Exit(1)
	}

	domains := []Domain{}
	if err := db.Find(&domains).Error; err != nil {
		log.Printf("FATAL DomainInit:FindAll: %s", err)
		os.Exit(1)
	}

	if len(domains) == 0 {
		domain := Domain{
			Name:      Def_Domain,
			CreatedBy: 1,
			UpdatedBy: 1,
		}
		if err := db.Create(&domain).Error; err != nil {
			log.Printf("FATAL DomainInit:Create: %s", err)
			os.Exit(1)
		}
	}
}

func (domain *Domain) DomainSetup(db *gorm.DB) {
	t, _ := i18n.Tfunc(Language)

	addresses := []Address{}
	if err := db.Where("domain_id = ?", domain.ID).Order("local_part").Find(&addresses).Error; err != nil {
		log.Printf("ERROR DomainSetup:Addresses: %s", err)
	}
	domain.Addresses = addresses

	domain.ConfirmDelete = fmt.Sprintf(t("delete_are_you_sure"), domain.Name)
}

func DomainFindByID(id int, db *gorm.DB) *Domain {
	domain := &Domain{}
	if err := db.First(domain, id).Error; err != nil {
		return nil
	}
	return domain
}

func DomainFindByName(name string, db *gorm.DB) *Domain {
	domain := &Domain{}
	if err := db.Where("name = ?", name).First(domain).Error; err != nil {
		return nil
	}
	return domain
}

func DomainFindAll(db *gorm.DB, name string) []Domain {
	log.Printf("DEBUG DomainFindAll: %s", name)

	domains := []Domain{}
	if err := db.Order("name").Find(&domains).Error; err != nil {
		log.Printf("ERROR DomainFindAll: %s", err)
	}

	for index, _ := range domains {
		domain := &domains[index]
		domain.DomainSetup(db)
		domain.Selected = (domain.Name == name)
	}

	return domains
}

func DomainCreate(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	log.Printf("INFO  CREATE /domain")

	db := OpenDB(true)
	defer CloseDB()

	ctx := AddressContext(w, r, "domain_create", true, db)
	if !ctx.LoggedIn {
		return
	}

	ctx.Domain = &Domain{
		ID:        0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	RenderHtml(w, r, "domain_edit", ctx)
}

func DomainEdit(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	t, _ := i18n.Tfunc(Language)
	id, _ := strconv.Atoi(ps.ByName("id"))
	log.Printf("INFO  EDIT /domain/%d", id)

	db := OpenDB(true)
	defer CloseDB()

	ctx := AddressContext(w, r, "domain_edit", true, db)
	if !ctx.LoggedIn {
		return
	}

	if ctx.Domain = DomainFindByID(id, db); ctx.Domain == nil {
		flash := fmt.Sprintf(t("flash_domain_not_found"), id)
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, HomeURL, http.StatusFound)
		return
	}
	ctx.Domain.DomainSetup(db)

	RenderHtml(w, r, "domain_edit", ctx)
}

func DomainUpdate(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	t, _ := i18n.Tfunc(Language)
	id, _ := strconv.Atoi(ps.ByName("id"))
	log.Printf("INFO  UPDATE /domain/%d", id)

	db := OpenDB(true)
	defer CloseDB()

	ctx := AddressContext(w, r, "domain_update", true, db)
	if !ctx.LoggedIn {
		return
	}

	name := r.FormValue("domain_name")

	if id == 0 {
		domain := &Domain{
			Name:      name,
			CreatedBy: ctx.CurrentAddress.ID,
			UpdatedBy: ctx.CurrentAddress.ID,
		}
		if err := db.Create(domain).Error; err != nil {
			flash := fmt.Sprintf(t("flash_error_text"), err.Error())
			if strings.Index(err.Error(), "UNIQUE") >= 0 {
				flash = fmt.Sprintf(t("flash_error_exists"), name)
			}
			SetFlash(w, F_ERROR, flash)
			http.Redirect(w, r, HomeURL, http.StatusFound)
			return
		}

		flash := fmt.Sprintf(t("flash_created"), domain.Name)
		SetFlash(w, F_INFO, flash)
		http.Redirect(w, r, HomeURL, http.StatusFound)
		return
	}

	domain := DomainFindByID(id, db)
	if domain == nil {
		flash := fmt.Sprintf(t("flash_domain_not_found"), id)
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, HomeURL, http.StatusFound)
		return
	}

	if domain.Name == name {
		http.Redirect(w, r, HomeURL, http.StatusFound)
		return
	}

	update := make(map[string]interface{})
	update["name"] = name
	update["updated_at"] = time.Now()
	update["updated_by"] = ctx.CurrentAddress.ID

	if err := db.Model(domain).Updates(update).Error; err != nil {
		flash := fmt.Sprintf(t("flash_error_text"), err.Error())
		if strings.Index(err.Error(), "UNIQUE") >= 0 {
			flash = fmt.Sprintf(t("flash_error_exists"), name)
		}
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, HomeURL, http.StatusFound)
		return
	}

	addresses := []Address{}
	if err := db.Where("domain_id = ?", domain.ID).Find(&addresses).Error; err != nil {
		flash := fmt.Sprintf(t("flash_error_text"), err.Error())
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, HomeURL, http.StatusFound)
		return
	}
	for index, _ := range addresses {
		address := &addresses[index]
		db.Model(address).Updates(Address{
			Email:      fmt.Sprintf("%s@%s", address.LocalPart, domain.Name),
			DomainName: domain.Name,
			UpdatedAt:  time.Now(),
			UpdatedBy:  ctx.CurrentAddress.ID,
		})
	}

	aliases := []Alias{}
	if err := db.Where("domain_id = ?", domain.ID).Find(&aliases).Error; err != nil {
		flash := fmt.Sprintf(t("flash_error_text"), err.Error())
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, HomeURL, http.StatusFound)
		return
	}
	for index, _ := range aliases {
		alias := &aliases[index]
		destination := AddressFindByID(alias.AddressID, db)
		if destination == nil {
			flash := fmt.Sprintf(t("flash_address_not_found"), alias.AddressID)
			SetFlash(w, F_ERROR, flash)
			http.Redirect(w, r, HomeURL, http.StatusFound)
			return
		}
		db.Model(alias).Updates(Alias{
			Email:       fmt.Sprintf("%s@%s", alias.LocalPart, domain.Name),
			Destination: destination.Email,
			DomainName:  domain.Name,
			UpdatedAt:   time.Now(),
			UpdatedBy:   ctx.CurrentAddress.ID,
		})
	}

	flash := fmt.Sprintf(t("flash_updated"), domain.Name)
	SetFlash(w, F_INFO, flash)
	http.Redirect(w, r, HomeURL, http.StatusFound)
}

func DomainDelete(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	t, _ := i18n.Tfunc(Language)
	id, _ := strconv.Atoi(ps.ByName("id"))
	log.Printf("INFO  DELETE /domain/%d", id)

	db := OpenDB(true)
	defer CloseDB()

	ctx := AddressContext(w, r, "domain_delete", true, db)
	if !ctx.LoggedIn {
		return
	}

	domain := DomainFindByID(id, db)
	if domain == nil {
		flash := fmt.Sprintf(t("flash_domain_not_found"), id)
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, HomeURL, http.StatusFound)
		return
	}
	name := domain.Name

	domain.DomainSetup(db)
	if len(domain.Addresses) > 0 {
		flash := fmt.Sprintf(t("flash_domain_not_empty"), name)
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, HomeURL, http.StatusFound)
		return
	}

	if err := db.Delete(domain).Error; err != nil {
		flash := fmt.Sprintf(t("flash_error_text"), err.Error())
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, HomeURL, http.StatusFound)
		return
	}

	flash := fmt.Sprintf(t("flash_deleted"), name)
	SetFlash(w, F_INFO, flash)
	http.Redirect(w, r, HomeURL, http.StatusFound)
}
