package contact_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/core/contact"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/stretchr/testify/require"
)

func TestContactService_Lifecycle(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	auditService := audit.NewService(querier)
	svc := contact.NewService(querier, auditService)

	actorID := pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}

	// 1. Validation errors
	_, err := svc.Create(ctx, actorID, contact.CreateContactInput{
		FirstName: "",
		LastName:  "Mustermann",
	})
	if err != contact.ErrInvalidContact {
		t.Fatalf("expected ErrInvalidContact for empty first name, got %v", err)
	}

	_, err = svc.Create(ctx, actorID, contact.CreateContactInput{
		FirstName: "Max",
		LastName:  "",
	})
	if err != contact.ErrInvalidContact {
		t.Fatalf("expected ErrInvalidContact for empty last name, got %v", err)
	}

	// 2. Create valid contact
	lat := 50.1109
	lon := 8.6821
	created, err := svc.Create(ctx, actorID, contact.CreateContactInput{
		FirstName:     "Dr. Michael",
		LastName:      "Weber",
		Email:         "michael.weber@weber-solar.de",
		Phone:         "+49 69 987654",
		Mobile:        "+49 171 1234567",
		Position:      "Geschäftsführer",
		LeadSource:    "Messe Frankfurt",
		AddressStreet: "Westhafen Tower 1",
		AddressZip:    "60327",
		AddressCity:   "Frankfurt",
		Latitude:      &lat,
		Longitude:     &lon,
	})
	if err != nil {
		t.Fatalf("failed to create contact: %v", err)
	}
	if created.FirstName != "Dr. Michael" || created.LastName != "Weber" {
		t.Fatalf("unexpected name: %s %s", created.FirstName, created.LastName)
	}

	// 3. Get contact by ID
	fetched, err := svc.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("failed to get contact: %v", err)
	}
	if fetched.ID != created.ID {
		t.Fatalf("expected ID %v, got %v", created.ID, fetched.ID)
	}

	// 4. Update contact
	updated, err := svc.Update(ctx, actorID, contact.UpdateContactInput{
		ID:        created.ID,
		FirstName: "Dr. Michael",
		LastName:  "Weber-Schulz",
		Email:     "m.weber@weber-solar.de",
	})
	if err != nil {
		t.Fatalf("failed to update contact: %v", err)
	}
	if updated.LastName != "Weber-Schulz" {
		t.Fatalf("unexpected updated last name: %s", updated.LastName)
	}

	// 5. List contacts
	contacts, err := svc.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("failed to list contacts: %v", err)
	}
	if len(contacts) == 0 {
		t.Fatalf("expected at least 1 contact")
	}

	// 6. Delete contact
	if err := svc.Delete(ctx, actorID, created.ID); err != nil {
		t.Fatalf("failed to delete contact: %v", err)
	}
}

func TestContactService_AutoGeocodingOnCreateAndPreservation(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	svc := contact.NewService(q, nil)

	// 1. Create with PLZ 10115 and nil coordinates -> coordinates automatically populated
	c, err := svc.Create(context.Background(), pgtype.UUID{}, contact.CreateContactInput{
		FirstName:   "Erika",
		LastName:    "Musterfrau",
		Email:       "erika@example.de",
		AddressZip:  "10115",
		AddressCity: "Berlin",
	})
	require.NoError(t, err)
	require.True(t, c.Latitude.Valid)
	require.True(t, c.Longitude.Valid)
	require.InDelta(t, 52.53, c.Latitude.Float64, 0.1)
	require.InDelta(t, 13.38, c.Longitude.Float64, 0.1)

	// 2. Create with manual latitude set -> manual coordinate is preserved, NOT overwritten
	manualLat := 49.0
	c2, err := svc.Create(context.Background(), pgtype.UUID{}, contact.CreateContactInput{
		FirstName:   "Hans",
		LastName:    "Manual",
		Email:       "hans@example.de",
		AddressZip:  "10115",
		AddressCity: "Berlin",
		Latitude:    &manualLat,
	})
	require.NoError(t, err)
	require.True(t, c2.Latitude.Valid)
	require.Equal(t, 49.0, c2.Latitude.Float64)
	require.False(t, c2.Longitude.Valid, "longitude was nil and latitude was set, so neither was auto-overwritten")

	// 3. Update contact: when coordinates are already present, changing PLZ does not overwrite them
	manualLon := 10.0
	c3, err := svc.Update(context.Background(), pgtype.UUID{}, contact.UpdateContactInput{
		ID:          c.ID,
		FirstName:   "Erika",
		LastName:    "Musterfrau",
		Email:       "erika@example.de",
		AddressZip:  "80331", // Munich PLZ
		AddressCity: "München",
		Latitude:    &manualLat,
		Longitude:   &manualLon,
	})
	require.NoError(t, err)
	require.Equal(t, 49.0, c3.Latitude.Float64, "preserved manual latitude")
	require.Equal(t, 10.0, c3.Longitude.Float64, "preserved manual longitude")
}

func TestContactService_ConsentAndEnergyFields(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	auditService := audit.NewService(querier)
	svc := contact.NewService(querier, auditService)

	actorID := pgtype.UUID{Bytes: [16]byte{1, 1, 1}, Valid: true}
	strom := 4500.50
	gas := 12000.00

	// 1. Create with consent
	c1, err := svc.Create(ctx, actorID, contact.CreateContactInput{
		FirstName:         "Max",
		LastName:          "Mustermann",
		Email:             "max@mustermann.de",
		Phone:             "+49 69 112233",
		ConsentPhone:      true,
		ConsentEmail:      false,
		StromverbrauchKwh: &strom,
		GasverbrauchKwh:   &gas,
		Zaehlernummer:     "1EMH12345678",
		EigentuemerStatus: "EIGENTUEMER",
	})
	require.NoError(t, err)
	require.True(t, c1.ConsentPhone)
	require.False(t, c1.ConsentEmail)
	require.True(t, c1.ConsentUpdatedAt.Valid)
	initialConsentTime := c1.ConsentUpdatedAt.Time

	// Energy fields verified
	require.Equal(t, "1EMH12345678", c1.Zaehlernummer.String)
	require.Equal(t, "EIGENTUEMER", c1.EigentuemerStatus.String)

	// 2. Update without consent change -> consent_updated_at must remain identical, audit log must mask PII
	c2, err := svc.Update(ctx, actorID, contact.UpdateContactInput{
		ID:                c1.ID,
		FirstName:         "Maximilian",
		LastName:          "Mustermann",
		Email:             "max.neu@mustermann.de",
		Phone:             "+49 69 998877",
		ConsentPhone:      true,
		ConsentEmail:      false,
		Zaehlernummer:     "1EMH12345678",
		EigentuemerStatus: "EIGENTUEMER",
	})
	require.NoError(t, err)
	require.Equal(t, initialConsentTime, c2.ConsentUpdatedAt.Time)

	// Verify that the UPDATE audit log masked email and phone (F-52)
	logs, err := querier.ListAuditLogsByEntity(ctx, db.ListAuditLogsByEntityParams{
		EntityType: "CONTACT",
		EntityID:   c1.ID,
		Limit:      10,
	})
	require.NoError(t, err)
	require.True(t, len(logs) >= 2)
	var updateOld, updateNew map[string]any
	json.Unmarshal(logs[0].OldValues, &updateOld)
	json.Unmarshal(logs[0].NewValues, &updateNew)
	require.NotEqual(t, "max.neu@mustermann.de", updateNew["email"])
	require.NotEqual(t, "+49 69 998877", updateNew["phone"])

	// 3. Update with consent change -> consent_updated_at changes, audit log recorded
	time.Sleep(10 * time.Millisecond)
	c3, err := svc.Update(ctx, actorID, contact.UpdateContactInput{
		ID:                c1.ID,
		FirstName:         "Maximilian",
		LastName:          "Mustermann",
		Email:             "max.neu@mustermann.de",
		ConsentPhone:      true,
		ConsentEmail:      true, // Changed!
		Zaehlernummer:     "1EMH12345678",
		EigentuemerStatus: "EIGENTUEMER",
	})
	require.NoError(t, err)
	require.True(t, c3.ConsentEmail)
	require.True(t, c3.ConsentUpdatedAt.Time.After(initialConsentTime))

	// Verify CONSENT_UPDATE audit log
	logsAfter, _ := querier.ListAuditLogsByEntity(ctx, db.ListAuditLogsByEntityParams{
		EntityType: "CONTACT",
		EntityID:   c1.ID,
		Limit:      10,
	})
	foundConsentAudit := false
	for _, l := range logsAfter {
		if l.Action == "CONSENT_UPDATE" {
			foundConsentAudit = true
			var newV map[string]any
			json.Unmarshal(l.NewValues, &newV)
			require.Equal(t, true, newV["consent_email"])
			require.Equal(t, true, newV["consent_phone"])
			break
		}
	}
	require.True(t, foundConsentAudit, "expected CONSENT_UPDATE audit log entry")
}
