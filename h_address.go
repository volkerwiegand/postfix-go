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
	OtherEmail    string
	Bcrypt        string
	Sha512        string
	Initial       string
	Admin         bool
	// Computed values
	Domain        *Domain
	Aliases       []Alias
	AliasList     string      `sql:"-"`
	ConfirmDelete string      `sql:"-"`
	Base_URL      string      `sql:"-"`
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
		email := fmt.Sprintf("%s@%s", local_part, domain.Name)
		address := Address{
			Email:      email,
			CreatedBy:  1,
			UpdatedBy:  1,
			LocalPart:  local_part,
			DomainName: domain.Name,
			DomainID:   domain.ID,
			Bcrypt:     PasswordBcrypt(email, t("password_default")),
			Sha512:     PasswordSha512(email, t("password_default")),
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
	t, _ := i18n.Tfunc(Language)

	domain := &Domain{}
	db.Find(domain, address.DomainID)
	address.Domain = domain

	aliases := []Alias{}
	if err := db.Where("address_id = ?", address.ID).Order("local_part").Find(&aliases).Error; err != nil {
		log.Printf("ERROR AddressSetup:Aliases: %s", err)
	}
	address.Aliases = aliases

	address.AliasList = ""
	for _, alias := range aliases {
		if address.AliasList != "" {
			address.AliasList += "\n"
		}
		address.AliasList += alias.LocalPart
	}

	address.ConfirmDelete = fmt.Sprintf(t("delete_are_you_sure"), address.Email)
	address.Base_URL = Base_URL
}

func AddressFindByID(id int, db *gorm.DB) *Address {
	address := &Address{}
	if err := db.First(address, id).Error; err != nil {
		return nil
	}
	return address
}

func AddressFindByEmail(email string, db *gorm.DB) *Address {
	address := &Address{}
	if err := db.Where("email = ?", email).First(address).Error; err != nil {
		return nil
	}
	return address
}

func AddressIsLoggedIn(r *http.Request, db *gorm.DB) (*Address, bool) {
	if uid := GetCookie(r, "address_id"); uid != "" {
		id, _ := strconv.Atoi(uid)
		if address := AddressFindByID(id, db); address != nil {
			//log.Printf("DEBUG is_logged_in as %s", address.Email)
			return address, true
		}
	}

	return nil, false
}

func AddressContext(w http.ResponseWriter, r *http.Request, title string, need_admin bool, db *gorm.DB) Context {
	t, _ := i18n.Tfunc(Language)

	ctx := Context{
		Title:          title,
		Base_URL:       Base_URL,
		CurrentAddress: &Address{},
		LoggedIn:       false,
	}
	if db == nil {
		return ctx
	}

	if address, ok := AddressIsLoggedIn(r, db); ok {
		if address.Admin == true || need_admin == false {
			ctx.CurrentAddress = address
			ctx.LoggedIn = true
			return ctx
		}

		SetFlash(w, F_ERROR, t("flash_forbidden"))
		http.Redirect(w, r, LogoutURL(), http.StatusFound)
		return ctx
	}

	SetFlash(w, F_ERROR, t("flash_need_login"))
	http.Redirect(w, r, LoginURL(), http.StatusFound)
	return ctx
}

func AddressCreate(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	t, _ := i18n.Tfunc(Language)
	log.Printf("INFO  GET %saddress", Base_URL)

	db := OpenDB(true)
	defer CloseDB()

	ctx := AddressContext(w, r, "address_create", true, db)
	if !ctx.LoggedIn {
		return
	}
	if ctx.CurrentAddress.Admin == false {
		flash := fmt.Sprintf(t("flash_forbidden"))
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, HomeURL(), http.StatusFound)
		return
	}

	ctx.Address = &Address{ID: 0, DomainName: Def_Domain, Admin: false}
	ctx.Domains = DomainFindAll(db, Def_Domain)

	RenderHtml(w, r, "address_edit", ctx)
}

func AddressEdit(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	t, _ := i18n.Tfunc(Language)
	id, _ := strconv.Atoi(ps.ByName("id"))
	log.Printf("INFO  GET %saddress/%d", Base_URL, id)

	db := OpenDB(true)
	defer CloseDB()

	ctx := AddressContext(w, r, "address_edit", true, db)
	if !ctx.LoggedIn {
		return
	}

	if ctx.Address = AddressFindByID(id, db); ctx.Address == nil {
		flash := fmt.Sprintf(t("flash_address_not_found"), id)
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, HomeURL(), http.StatusFound)
		return
	}
	ctx.Address.AddressSetup(db)
	ctx.Domains = DomainFindAll(db, ctx.Address.DomainName)

	RenderHtml(w, r, "address_edit", ctx)
}

func AddressUpdate(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	t, _ := i18n.Tfunc(Language)
	id, _ := strconv.Atoi(ps.ByName("id"))
	log.Printf("INFO  POST %saddress/%d", Base_URL, id)

	db := OpenDB(true)
	defer CloseDB()

	ctx := AddressContext(w, r, "address_update", true, db)
	if !ctx.LoggedIn {
		return
	}

	domain_name := r.FormValue("address_domain_name")
	domain := &Domain{}
	if err := db.Where("name = ?", domain_name).First(domain).Error; err != nil {
		flash := fmt.Sprintf(t("flash_error_text"), err.Error())
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, HomeURL(), http.StatusFound)
		return
	}

	local_part  := r.FormValue("address_local_part")
	email       := fmt.Sprintf("%s@%s", local_part, domain.Name)
	admin       := r.FormValue("address_admin")
	other_email := r.FormValue("address_other_email")
	//log.Printf("DEBUG LocalPart=%s DomainName=%s Admin=%s", local_part, domain.Name, admin)

	alias_names := []string{}
	for _, alias_name := range strings.Split(r.FormValue("address_alias_list"), "\n") {
		alias_name = strings.TrimSpace(alias_name)
		if alias_name == "" || alias_name == local_part {
			continue
		}
		if flash := AliasCheck(alias_name, domain.Name, id, db); flash != "" {
			SetFlash(w, F_ERROR, flash)
			http.Redirect(w, r, HomeURL(), http.StatusFound)
			return
		}
		log.Printf("INFO  Alias: %v", alias_name)
		alias_names = append(alias_names, alias_name)
	}
	//log.Printf("DEBUG AliasNames=%v", alias_names)

	if id == 0 {
		if flash := AliasCheck(local_part, domain.Name, id, db); flash != "" {
			SetFlash(w, F_ERROR, flash)
			http.Redirect(w, r, HomeURL(), http.StatusFound)
			return
		}

		address := &Address{
			LocalPart:  local_part,
			DomainName: domain.Name,
			Email:      email,
			OtherEmail: other_email,
			DomainID:   domain.ID,
			Admin:      admin == "yes",
			CreatedBy:  ctx.CurrentAddress.ID,
			UpdatedBy:  ctx.CurrentAddress.ID,
		}
		if err := db.Create(address).Error; err != nil {
			flash := fmt.Sprintf(t("flash_error_text"), err.Error())
			if strings.Index(err.Error(), "UNIQUE") >= 0 {
				flash = fmt.Sprintf(t("flash_error_exists"), email)
			}
			SetFlash(w, F_ERROR, flash)
			http.Redirect(w, r, HomeURL(), http.StatusFound)
			return
		}

		for _, alias_name := range alias_names {
			if flash := AliasCreate(address, alias_name, db); flash != "" {
				SetFlash(w, F_ERROR, flash)
				http.Redirect(w, r, HomeURL(), http.StatusFound)
				return
			}
		}

		flash := fmt.Sprintf(t("flash_created"), address.Email)
		SetFlash(w, F_INFO, flash)
		http.Redirect(w, r, HomeURL(), http.StatusFound)
		return
	}

	address := AddressFindByID(id, db)
	if address == nil {
		flash := fmt.Sprintf(t("flash_address_not_found"), id)
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, HomeURL(), http.StatusFound)
		return
	}

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
	if address.OtherEmail != other_email {
		update["other_email"] = other_email
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
		http.Redirect(w, r, HomeURL(), http.StatusFound)
		return
	}

	db.Where("address_id = ?", address.ID).Delete(&Alias{})
	for _, alias_name := range alias_names {
		if flash := AliasCreate(address, alias_name, db); flash != "" {
			SetFlash(w, F_ERROR, flash)
			http.Redirect(w, r, HomeURL(), http.StatusFound)
			return
		}
	}

	flash := fmt.Sprintf(t("flash_updated"), address.Email)
	SetFlash(w, F_INFO, flash)
	http.Redirect(w, r, HomeURL(), http.StatusFound)
}

func AddressPrint(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	t, _ := i18n.Tfunc(Language)
	id, _ := strconv.Atoi(ps.ByName("id"))
	log.Printf("INFO  GET %saddress/%d/print", Base_URL, id)

	db := OpenDB(true)
	defer CloseDB()

	ctx := AddressContext(w, r, "address_print", true, db)
	if !ctx.LoggedIn {
		return
	}

	if ctx.Address = AddressFindByID(id, db); ctx.Address == nil {
		flash := fmt.Sprintf(t("flash_address_not_found"), id)
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, HomeURL(), http.StatusFound)
		return
	}

	initial := PasswordRandom(10)
	update := make(map[string]interface{})
	update["initial"] = PasswordBcrypt(ctx.Address.Email, initial)
	update["updated_at"] = time.Now()
	update["updated_by"] = ctx.CurrentAddress.ID

	if err := db.Model(ctx.Address).Updates(update).Error; err != nil {
		flash := fmt.Sprintf(t("flash_error_text"), err.Error())
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, HomeURL(), http.StatusFound)
		return
	}

	PasswordLetter(w, ctx, initial)
}

func AddressDelete(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	t, _ := i18n.Tfunc(Language)
	id, _ := strconv.Atoi(ps.ByName("id"))
	log.Printf("INFO  GET %saddress/%d/delete", Base_URL, id)

	db := OpenDB(true)
	defer CloseDB()

	ctx := AddressContext(w, r, "address_delete", true, db)
	if !ctx.LoggedIn {
		return
	}
	if id == ctx.CurrentAddress.ID {
		flash := fmt.Sprintf(t("flash_forbidden"))
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, HomeURL(), http.StatusFound)
		return
	}

	if ctx.Address = AddressFindByID(id, db); ctx.Address == nil {
		flash := fmt.Sprintf(t("flash_address_not_found"), id)
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, HomeURL(), http.StatusFound)
		return
	}

	if err := db.Where("address_id = ?", ctx.Address.ID).Delete(&Alias{}).Error; err != nil {
		flash := fmt.Sprintf(t("flash_error_text"), err.Error())
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, HomeURL(), http.StatusFound)
		return
	}

	email := ctx.Address.Email
	if err := db.Delete(ctx.Address).Error; err != nil {
		flash := fmt.Sprintf(t("flash_error_text"), err.Error())
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, HomeURL(), http.StatusFound)
		return
	}

	flash := fmt.Sprintf(t("flash_deleted"), email)
	SetFlash(w, F_INFO, flash)
	http.Redirect(w, r, HomeURL(), http.StatusFound)
}
