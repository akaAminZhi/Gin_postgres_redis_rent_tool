package db

import (
	"Gin_postgres_redis_rent_tool/models"
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func ConnectDB() *gorm.DB {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_PORT"),
	)

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database: ", err)
	}

	// err = DB.AutoMigrate(&models.User{}, &models.Credential{}, &models.Invite{}, &models.Item{}, &models.Loan{})
	err = Migrate(DB)
	if err != nil {
		log.Fatal("Failed to migrate models: ", err)
	}
	log.Println("Database connected")
	return DB
}

func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&models.User{}, &models.Credential{}, &models.Invite{}, &models.Item{}, &models.Loan{}, &models.UnlockLog{}); err != nil {
		return err
	}

	// 同一物品最多一条“未归还”
	if err := db.Exec(fmt.Sprintf(`
	  CREATE UNIQUE INDEX IF NOT EXISTS %s_one_open_per_item
	  ON %s (item_id)
	  WHERE returned_at IS NULL;
	`, models.LoanTable, models.LoanTable)).Error; err != nil {
		return err
	}

	// 查询当前借用更快
	if err := db.Exec(fmt.Sprintf(`
	  CREATE INDEX IF NOT EXISTS %s_open_item_borrowedat_desc
	  ON %s (item_id, borrowed_at DESC)
	  WHERE returned_at IS NULL;
	`, models.LoanTable, models.LoanTable)).Error; err != nil {
		return err
	}

	return nil
}
