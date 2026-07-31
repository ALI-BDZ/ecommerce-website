package handlers

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/yourorg/ecommerce/database"
)

func LogActivity(action, entityType, entityID, details string) {
	if database.DB == nil {
		return
	}
	data := map[string]interface{}{
		"entity_type": entityType,
		"entity_id":   entityID,
		"details":     details,
		"timestamp":   time.Now().Format(time.RFC3339),
	}
	dataJSON, _ := json.Marshal(data)
	_, err := database.DB.Exec(context.Background(),
		`INSERT INTO activity_logs (action, entity_type, entity_id, data) VALUES ($1,$2,$3,$4)`,
		action, entityType, entityID, string(dataJSON))
	if err != nil {
		log.Printf("warning: failed to log activity: %v", err)
	}
}

func CreateNotification(notifType, title, body string, data map[string]interface{}) {
	if database.DB == nil {
		return
	}
	dataJSON, _ := json.Marshal(data)
	_, err := database.DB.Exec(context.Background(),
		`INSERT INTO notifications (type, title, body, data) VALUES ($1,$2,$3,$4)`,
		notifType, title, body, string(dataJSON))
	if err != nil {
		log.Printf("warning: failed to create notification: %v", err)
	}
}

func RecordDeliveryHistory(customerID, orderID, event, notes string) {
	if database.DB == nil {
		return
	}
	_, err := database.DB.Exec(context.Background(),
		`INSERT INTO customer_delivery_history (customer_id, order_id, event, notes) VALUES ($1,$2,$3,$4)`,
		customerID, orderID, event, notes)
	if err != nil {
		log.Printf("warning: failed to record delivery history: %v", err)
	}
}

func CleanupExpiredPacks() {
	if database.DB == nil {
		return
	}
	tag, err := database.DB.Exec(context.Background(), `DELETE FROM packs WHERE expires_at < now()`)
	if err != nil {
		log.Printf("warning: pack cleanup failed: %v", err)
		return
	}
	if n := tag.RowsAffected(); n > 0 {
		log.Printf("cleaned up %d expired packs", n)
	}
}

func StartBackgroundCleanup() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			CleanupExpiredPacks()
			CheckLowStock()
		}
	}()
}
