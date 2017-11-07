package main

import (
	"os"
	"log"
	"fmt"
	"time"
	"strings"
	"github.com/jinzhu/gorm"
	"github.com/nicksnyder/go-i18n/i18n"
)

type Alias struct {
	ID            int         `gorm:"primary_key"`
	Email         string      `gorm:"unique_index"`
	Destination   string
	CreatedAt     time.Time
	CreatedBy     int         `gorm:"index"`
	UpdatedAt     time.Time
	UpdatedBy     int         `gorm:"index"`
	LocalPart     string      `gorm:"index"`
	DomainName    string
	DomainID      int         `gorm:"index"`
	AddressID     int         `gorm:"index"`
	// Computed values
	Domain        *Domain
	Address       *Address
}

func AliasInit() {
	db := OpenDB(true)
	defer CloseDB()

	if err := db.AutoMigrate(&Alias{}).Error; err != nil {
		log.Printf("FATAL AliasInit:AutoMigrate: %s", err)
		os.Exit(1)
	}

	aliases := []Alias{}
	if err := db.Find(&aliases).Error; err != nil {
		log.Printf("FATAL AliasInit:FindAll: %s", err)
		os.Exit(1)
	}
}

func AliasFindByID(id int, db *gorm.DB) *Alias {
	alias := &Alias{}
	if err := db.First(alias, id).Error; err != nil {
		return nil
	}
	return alias
}

func AliasFindByEmail(email string, db *gorm.DB) *Alias {
	alias := &Alias{}
	if err := db.Where("email = ?", email).First(alias).Error; err != nil {
		return nil
	}
	return alias
}

func AliasCheck(local_part, domain_name string, destination_id int, db *gorm.DB) string {
	t, _ := i18n.Tfunc(Language)

	email := fmt.Sprintf("%s@%s", local_part, domain_name)

	if destination_id != 0 {
		destination := AddressFindByID(destination_id, db)
		if destination == nil {
			return fmt.Sprintf(t("flash_address_not_found"), destination_id)
		}

		if address := AddressFindByEmail(email, db); address != nil {
			return fmt.Sprintf(t("flash_error_exists"), email)
		}
	}

	if alias := AliasFindByEmail(email, db); alias != nil {
		if alias.AddressID == destination_id {
			return ""
		}
		return fmt.Sprintf(t("flash_error_exists"), email)
	}

	return ""
}

func AliasCreate(destination *Address, local_part string, db *gorm.DB) string {
	t, _ := i18n.Tfunc(Language)

	email := fmt.Sprintf("%s@%s", local_part, destination.DomainName)
	log.Printf("INFO  creating alias %s for %s", email, destination.Email)

	alias := Alias{
		Email:       email,
		Destination: destination.Email,
		CreatedBy:   destination.CreatedBy,
		UpdatedBy:   destination.UpdatedBy,
		LocalPart:   local_part,
		DomainName:  destination.DomainName,
		DomainID:    destination.DomainID,
		AddressID:   destination.ID,
	}
	if err := db.Create(&alias).Error; err != nil {
		log.Printf("ERROR AliasCreate: %s", err)
		flash := fmt.Sprintf(t("flash_error_text"), err.Error())
		if strings.Index(err.Error(), "UNIQUE") >= 0 {
			flash = fmt.Sprintf(t("flash_error_exists"), email)
		}
		return flash
	}

	return ""
}
