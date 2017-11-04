package main

import (
	"os"
	"log"
	"fmt"
	"time"
	_ "strings"
	_ "strconv"
	_ "net/http"
	_ "github.com/julienschmidt/httprouter"
	"github.com/jinzhu/gorm"
	_ "github.com/nicksnyder/go-i18n/i18n"
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

func AliasCreate(destination *Address, local_part string, db *gorm.DB, creator_id int) {
	email := fmt.Sprintf("%s@%s", local_part, destination.DomainName)
	log.Printf("INFO  creating alias %s for %s", email, destination.Email)

	alias := Alias{
		Email:       email,
		Destination: destination.Email,
		CreatedBy:   creator_id,
		UpdatedBy:   creator_id,
		LocalPart:   local_part,
		DomainName:  destination.DomainName,
		DomainID:    destination.DomainID,
		AddressID:   destination.ID,
	}
	if err := db.Create(&alias).Error; err != nil {
		log.Printf("ERROR AliasCreate: %s", err)
	}
}
