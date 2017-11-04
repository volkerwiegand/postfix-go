package main

import (
	"os"
	"log"
	"fmt"
	"time"
	"strings"
	"strconv"
	"net/http"
	"golang.org/x/crypto/bcrypt"
	"github.com/julienschmidt/httprouter"
	"github.com/jinzhu/gorm"
	"github.com/nicksnyder/go-i18n/i18n"
)

type Address struct {
	ID            int         `gorm:"primary_key"`
	Email         string      `gorm:"unique_index"`
	CreatedAt     time.Time
	CreatedBy     int         `gorm:"index"`
	UpdatedAt     time.Time
	UpdatedBy     int         `gorm:"index"`
	LocalPart     string      `gorm:"index"`
	DomainName    string
	DomainID      int         `gorm:"index"`
	Password      string
	FirstPass     string
	Admin         bool
	// Computed values
	Domain        *Domain
	Aliases       []Alias
}

func AddressInit() {
	t, _ := i18n.Tfunc(Language)

	db := OpenDB(true)
	defer CloseDB()

	if err := db.AutoMigrate(&Address{}).Error; err != nil {
		log.Printf("FATAL AddressInit:AutoMigrate: %s", err)
		os.Exit(1)
	}

	addresses := []Address{}
	if err := db.Find(&addresses).Error; err != nil {
		log.Printf("FATAL AddressInit:FindAll: %s", err)
		os.Exit(1)
	}

	if len(addresses) == 0 {
		local_part := t("address_local_part_default")
		domain := DomainFindByName(Def_Domain, db)
		address := Address{
			Email:      fmt.Sprintf("%s@%s", local_part, domain.Name),
			CreatedBy:  1,
			UpdatedBy:  1,
			LocalPart:  local_part,
			DomainName: domain.Name,
			DomainID:   domain.ID,
			Password:   AddressPassword(t("address_password_default")),
			Admin:      true,
		}
		if err := db.Create(&address).Error; err != nil {
			log.Printf("FATAL AddressInit:Address: %s", err)
			os.Exit(1)
		}

		alias_parts := []string{"hostmaster", "postmaster", "webmaster"}
		for _, alias_part := range alias_parts {
			alias := Alias{
				Email:       fmt.Sprintf("%s@%s", alias_part, domain.Name),
				Destination: address.Email,
				CreatedBy:   1,
				UpdatedBy:   1,
				LocalPart:   alias_part,
				DomainName:  domain.Name,
				DomainID:    domain.ID,
				AddressID:   address.ID,
			}
			if err := db.Create(&alias).Error; err != nil {
				log.Printf("FATAL AddressInit:Alias: %s", err)
				os.Exit(1)
			}
		}
	}
}

func (address *Address) AddressSetup(db *gorm.DB) {
	domain := &Domain{}
	db.Find(domain, address.DomainID)
	address.Domain = domain

	aliases := []Alias{}
	if err := db.Where("address_id = ?", address.ID).Order("local_part").Find(&aliases).Error; err != nil {
		log.Printf("ERROR AddressSetup:Aliases: %s", err)
	}
	address.Aliases = aliases
}

func AddressPassword(password string) string {
	pswd := []byte(password)
	hash, _ := bcrypt.GenerateFromPassword(pswd, bcrypt.DefaultCost)
	return string(hash)
}

func AddressFindByID(id int, db *gorm.DB) *Address {
	address := &Address{}
	if err := db.First(address, id).Error; err != nil {
		log.Printf("ERROR address %d not found", id)
		return nil
	}
	address.AddressSetup(db)
	return address
}

func AddressFindByEmail(email string, db *gorm.DB) *Address {
	address := &Address{}
	if err := db.Where("email = ?", email).First(address).Error; err != nil {
		log.Printf("ERROR email %s not found (%s)", email, err)
		return nil
	}
	address.AddressSetup(db)
	return address
}

func AddressIsLoggedIn(r *http.Request, db *gorm.DB) (*Address, bool) {
	if uid := GetCookie(r, "address_id"); uid != "" {
		id, _ := strconv.Atoi(uid)
		if address := AddressFindByID(id, db); address != nil {
			//log.Printf("INFO  is_logged_in as %s", address.Email)
			return address, true
		}
	}

	return nil, false
}

func AddressContext(w http.ResponseWriter, r *http.Request, title string, admin bool, db *gorm.DB) Context {
	t, _ := i18n.Tfunc(Language)
	ctx := Context{Title: title, CurrentAddress: &Address{}, LoggedIn: false}

	if address, ok := AddressIsLoggedIn(r, db); ok {
		if address.Admin {
			ctx.CurrentAddress = address
			ctx.LoggedIn = true
			return ctx
		}

		if admin {
			SetFlash(w, F_ERROR, t("flash_forbidden"))
			http.Redirect(w, r, LogoutURL, http.StatusFound)
			return ctx
		}
	}

	SetFlash(w, F_ERROR, t("flash_need_login"))
	http.Redirect(w, r, LoginURL, http.StatusFound)
	return ctx
}

func AddressCreate(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	t, _ := i18n.Tfunc(Language)
	log.Printf("INFO  CREATE /address")

	db := OpenDB(true)
	defer CloseDB()

	ctx := AddressContext(w, r, "address_create", true, db)
	if !ctx.LoggedIn {
		return
	}
	if !ctx.CurrentAddress.Admin {
		flash := fmt.Sprintf(t("flash_forbidden"))
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, HomeURL, http.StatusFound)
		return
	}

	domain := *DomainFindByName(Def_Domain, db)
	ctx.Address = &Address{
		ID:         0,
		DomainName: domain.Name,
		Admin:      false,
	}

	ctx.Domains = DomainFindAll(db, Def_Domain)

	RenderHtml(w, r, "address_edit", ctx)
}

func AddressEdit(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	t, _ := i18n.Tfunc(Language)
	id, _ := strconv.Atoi(ps.ByName("id"))
	log.Printf("INFO  EDIT /address/%d", id)

	db := OpenDB(true)
	defer CloseDB()

	ctx := AddressContext(w, r, "address_edit", false, db)
	if !ctx.LoggedIn {
		return
	}

	if !ctx.CurrentAddress.Admin {
		// Non-Admins can do nothing but change their own poassword
		ctx.Address = AddressFindByID(ctx.CurrentAddress.ID, db)
		ctx.Domains = DomainFindAll(db, ctx.Address.DomainName)
		RenderHtml(w, r, "address_password", ctx)
		return
	}

	if ctx.Address = AddressFindByID(id, db); ctx.Address == nil {
		flash := fmt.Sprintf(t("flash_address_not_found"), id)
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, HomeURL, http.StatusFound)
		return
	}

	ctx.Domains = DomainFindAll(db, ctx.Address.DomainName)

	RenderHtml(w, r, "address_edit", ctx)
}

func AddressUpdate(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	t, _ := i18n.Tfunc(Language)
	id, _ := strconv.Atoi(ps.ByName("id"))
	log.Printf("INFO  UPDATE /address/%d", id)

	db := OpenDB(true)
	defer CloseDB()

	ctx := AddressContext(w, r, "address_update", false, db)
	if !ctx.LoggedIn {
		return
	}

	if !ctx.CurrentAddress.Admin {
		password := r.FormValue("address_password")
		if password != "" {
			password = AddressPassword(password)
			if err := db.Model(ctx.CurrentAddress).Update("password", password).Error; err != nil {
				flash := fmt.Sprintf(t("flash_error_text"), err.Error())
				SetFlash(w, F_ERROR, flash)
			} else {
				flash := fmt.Sprintf(t("flash_updated"), t("address_password"))
				SetFlash(w, F_INFO, flash)
			}
		}
		http.Redirect(w, r, LogoutURL, http.StatusFound)
		return
	}

	local_part  := r.FormValue("address_local_part")
	domain_name := r.FormValue("address_domain_name")
	domain      := DomainFindByName(domain_name, db)
	email       := fmt.Sprintf("%s@%s", local_part, domain.Name)
	admin       := r.FormValue("address_admin")
	password    := r.FormValue("address_password")
	//log.Printf("INFO  LocalPart=%s DomainName=%s Admin=%s", local_part, domain.Name, admin)

	if id == 0 {
		address := Address{
			LocalPart:  local_part,
			DomainName: domain.Name,
			Email:      email,
			DomainID:   domain.ID,
			Admin:      admin == "yes",
			Password:   AddressPassword(password),
			CreatedBy:  ctx.CurrentAddress.ID,
			UpdatedBy:  ctx.CurrentAddress.ID,
		}
		if err := db.Create(&address).Error; err != nil {
			flash := fmt.Sprintf(t("flash_error_text"), err.Error())
			if strings.Index(err.Error(), "UNIQUE") >= 0 {
				flash = fmt.Sprintf(t("flash_error_exists"), email)
			}
			SetFlash(w, F_ERROR, flash)
		} else {
			flash := fmt.Sprintf(t("flash_created"), address.Email)
			SetFlash(w, F_INFO, flash)
		}
	} else {
		address := AddressFindByID(id, db)
		if address == nil {
			flash := fmt.Sprintf(t("flash_address_not_found"), id)
			SetFlash(w, F_ERROR, flash)
		} else {
			update := make(map[string]interface{})

			if address.LocalPart != local_part {
				update["local_part"] = local_part
			}
			if address.DomainID != domain.ID {
				update["domain_name"] = domain.Name
				update["domain_id"]   = domain.ID
			}
			if address.Email != email {
				update["email"] = email
			}
			if password != "" {
				update["password"] = AddressPassword(password)
			}
			if admin == "yes" {
				if address.Admin == false {
					update["admin"] = true
				}
			} else {
				if address.Admin == true {
					update["admin"] = false
				}
			}
			update["updated_at"] = time.Now()
			update["updated_by"] = ctx.CurrentAddress.ID

			if err := db.Model(address).Updates(update).Error; err != nil {
				flash := fmt.Sprintf(t("flash_error_text"), err.Error())
				if strings.Index(err.Error(), "UNIQUE") >= 0 {
					flash = fmt.Sprintf(t("flash_error_exists"), email)
				}
				SetFlash(w, F_ERROR, flash)
			} else {
				flash := fmt.Sprintf(t("flash_updated"), address.Email)
				SetFlash(w, F_INFO, flash)
			}
		}
	}

	http.Redirect(w, r, HomeURL, http.StatusFound)
}

func AddressDelete(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	t, _ := i18n.Tfunc(Language)
	id, _ := strconv.Atoi(ps.ByName("id"))
	log.Printf("INFO  DELETE /address/%d", id)

	db := OpenDB(true)
	defer CloseDB()

	ctx := AddressContext(w, r, "address_delete", true, db)
	if !ctx.LoggedIn {
		return
	}
	if id == ctx.CurrentAddress.ID {
		flash := fmt.Sprintf(t("flash_forbidden"), id)
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, HomeURL, http.StatusFound)
		return
	}

	address := AddressFindByID(id, db)
	if address == nil {
		flash := fmt.Sprintf(t("flash_address_not_found"), id)
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, HomeURL, http.StatusFound)
		return
	}
	email := address.Email

	// TODO delete all aliases

	if err := db.Delete(address).Error; err != nil {
		flash := fmt.Sprintf(t("flash_error_text"), err.Error())
		SetFlash(w, F_ERROR, flash)
	} else {
		flash := fmt.Sprintf(t("flash_deleted"), email)
		SetFlash(w, F_INFO, flash)
	}

	http.Redirect(w, r, HomeURL, http.StatusFound)
}
