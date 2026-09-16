package connectors

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/contact"
	"github.com/openlocalcrm/openlocalcrm/internal/core/deal"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

var (
	ErrInvalidWebhookPayload = errors.New("invalid webhook payload")
	ErrUnauthorizedConnector = errors.New("unauthorized connector token")
)

type LeadIntakePayload struct {
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	Street      string `json:"street"`
	City        string `json:"city"`
	Zip         string `json:"zip"`
	DealTitle   string `json:"deal_title"`
	DealValue   string `json:"deal_value"`
	Source      string `json:"source"`
	SetterNotes string `json:"setter_notes"`
}

type Engine struct {
	contactSvc *contact.Service
	dealSvc    *deal.Service
	apiToken   string
}

func NewEngine(contactSvc *contact.Service, dealSvc *deal.Service, apiToken string) *Engine {
	return &Engine{
		contactSvc: contactSvc,
		dealSvc:    dealSvc,
		apiToken:   apiToken,
	}
}

func (e *Engine) IngestLead(ctx context.Context, token string, payload LeadIntakePayload) (db.Contact, error) {
	if e.apiToken == "" || token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(e.apiToken)) != 1 {
		return db.Contact{}, ErrUnauthorizedConnector
	}

	if strings.TrimSpace(payload.LastName) == "" {
		return db.Contact{}, ErrInvalidWebhookPayload
	}

	var systemActor pgtype.UUID
	_ = systemActor.Scan("00000000-0000-0000-0000-000000000001")

	// 1. Create Contact
	c, err := e.contactSvc.Create(ctx, systemActor, contact.CreateContactInput{
		FirstName:     payload.FirstName,
		LastName:      payload.LastName,
		Email:         payload.Email,
		Phone:         payload.Phone,
		AddressStreet: payload.Street,
		AddressCity:   payload.City,
		AddressZip:    payload.Zip,
	})
	if err != nil {
		return db.Contact{}, err
	}

	// 2. If Deal Title is provided, create associated Deal
	if payload.DealTitle != "" && e.dealSvc != nil {
		var val pgtype.Numeric
		_ = val.Scan(payload.DealValue)

		_, _ = e.dealSvc.Create(ctx, systemActor, deal.CreateDealInput{
			ContactID:   &c.ID,
			Title:       payload.DealTitle,
			Value:       val,
			Stage:       "LEAD",
			Probability: 25,
		})
	}

	return c, nil
}
