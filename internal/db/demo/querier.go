package demo

import (
	"context"
	"errors"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

var (
	ErrNotFound = errors.New("record not found")
)

type InMemoryQuerier struct {
	mu            sync.RWMutex
	users         map[string]db.User
	companies     map[string]db.Company
	contacts      map[string]db.Contact
	deals         map[string]db.Deal
	todos         map[string]db.Todo
	notifications map[string]db.Notification
	auditLogs     []db.AuditLog
	emailAccounts map[string]db.EmailAccount
	emailMessages map[string]db.EmailMessage
	emailAttachs  map[string]db.EmailAttachment
	refreshTokens map[string]db.RefreshToken
}

func parseUUID(s string) pgtype.UUID {
	u, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{Valid: false}
	}
	return pgtype.UUID{Bytes: u, Valid: true}
}

func newUUID() pgtype.UUID {
	return pgtype.UUID{Bytes: uuid.New(), Valid: true}
}

func uuidToStr(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return uuid.UUID(u.Bytes).String()
}

func strToText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func nowTimestamptz() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
}

func NewInMemoryQuerier() *InMemoryQuerier {
	q := &InMemoryQuerier{
		users:         make(map[string]db.User),
		companies:     make(map[string]db.Company),
		contacts:      make(map[string]db.Contact),
		deals:         make(map[string]db.Deal),
		todos:         make(map[string]db.Todo),
		notifications: make(map[string]db.Notification),
		auditLogs:     make([]db.AuditLog, 0),
		emailAccounts: make(map[string]db.EmailAccount),
		emailMessages: make(map[string]db.EmailMessage),
		emailAttachs:  make(map[string]db.EmailAttachment),
		refreshTokens: make(map[string]db.RefreshToken),
	}

	hash, err := auth.HashPassword("demo123")
	if err != nil {
		// Fallback hash if argon2 fails
		hash = "$argon2id$v=19$m=65536,t=3,p=2$demo$fakehash"
	}

	adminID := parseUUID("11111111-1111-1111-1111-111111111111")
	vertriebID := parseUUID("22222222-2222-2222-2222-222222222222")

	q.users[uuidToStr(adminID)] = db.User{
		ID:           adminID,
		Email:        "admin@openlocalcrm.local",
		PasswordHash: hash,
		FirstName:    "Max",
		LastName:     "Vertriebsleiter",
		Role:         "ADMIN",
		Status:       "ACTIVE",
		CreatedAt:    nowTimestamptz(),
		UpdatedAt:    nowTimestamptz(),
	}

	q.users[uuidToStr(vertriebID)] = db.User{
		ID:           vertriebID,
		Email:        "vertrieb@openlocalcrm.local",
		PasswordHash: hash,
		FirstName:    "Felix",
		LastName:     "Setter",
		Role:         "BENUTZER",
		Status:       "ACTIVE",
		CreatedAt:    nowTimestamptz(),
		UpdatedAt:    nowTimestamptz(),
	}

	// Companies
	comp1ID := parseUUID("33333333-3333-3333-3333-333333333331")
	comp2ID := parseUUID("33333333-3333-3333-3333-333333333332")
	q.companies[uuidToStr(comp1ID)] = db.Company{
		ID:             comp1ID,
		Name:           "Weber Maschinenbau GmbH",
		Domain:         strToText("weber-maschinenbau.de"),
		Phone:          strToText("+49 711 555010"),
		Email:          strToText("info@weber-maschinenbau.de"),
		AddressStreet:  strToText("Industriestraße 14"),
		AddressZip:     strToText("70565"),
		AddressCity:    strToText("Stuttgart"),
		AddressCountry: strToText("Deutschland"),
		CreatedAt:      nowTimestamptz(),
		UpdatedAt:      nowTimestamptz(),
	}
	q.companies[uuidToStr(comp2ID)] = db.Company{
		ID:             comp2ID,
		Name:           "Bäckerei Schmidt e.K.",
		Domain:         strToText("schmidt-back.de"),
		Phone:          strToText("+49 731 223344"),
		Email:          strToText("kontakt@schmidt-back.de"),
		AddressStreet:  strToText("Marktplatz 4"),
		AddressZip:     strToText("89073"),
		AddressCity:    strToText("Ulm"),
		AddressCountry: strToText("Deutschland"),
		CreatedAt:      nowTimestamptz(),
		UpdatedAt:      nowTimestamptz(),
	}

	// Contacts
	cont1ID := parseUUID("44444444-4444-4444-4444-444444444441")
	cont2ID := parseUUID("44444444-4444-4444-4444-444444444442")
	q.contacts[uuidToStr(cont1ID)] = db.Contact{
		ID:                cont1ID,
		CompanyID:         comp1ID,
		FirstName:         "Florian",
		LastName:          "Weber",
		Email:             strToText("f.weber@weber-maschinenbau.de"),
		Phone:             strToText("+49 711 555019"),
		Position:          strToText("Geschäftsführer"),
		LeadSource:        strToText("Messe Intersolar"),
		AddressStreet:     strToText("Industriestraße 14"),
		AddressZip:        strToText("70565"),
		AddressCity:       strToText("Stuttgart"),
		ConsentPhone:      true,
		ConsentEmail:      true,
		StromverbrauchKwh: pgtype.Numeric{Int: big.NewInt(45000), Valid: true},
		Zaehlernummer:     strToText("1EMH004512998"),
		EigentuemerStatus: strToText("Eigentümer"),
		CreatedAt:         nowTimestamptz(),
		UpdatedAt:         nowTimestamptz(),
	}
	q.contacts[uuidToStr(cont2ID)] = db.Contact{
		ID:                cont2ID,
		FirstName:         "Sabine",
		LastName:          "Mustermann",
		Email:             strToText("sabine.mustermann@gmail.com"),
		Phone:             strToText("+49 171 1234567"),
		LeadSource:        strToText("D2D Haustür"),
		AddressStreet:     strToText("Sonnenhang 12"),
		AddressZip:        strToText("73728"),
		AddressCity:       strToText("Esslingen"),
		ConsentPhone:      true,
		ConsentEmail:      true,
		StromverbrauchKwh: pgtype.Numeric{Int: big.NewInt(4200), Valid: true},
		Zaehlernummer:     strToText("1STG998877665"),
		EigentuemerStatus: strToText("Eigentümer"),
		CreatedAt:         nowTimestamptz(),
		UpdatedAt:         nowTimestamptz(),
	}

	// Deals
	deal1ID := parseUUID("55555555-5555-5555-5555-555555555551")
	deal2ID := parseUUID("55555555-5555-5555-5555-555555555552")
	q.deals[uuidToStr(deal1ID)] = db.Deal{
		ID:          deal1ID,
		Title:       "PV-Gewerbedach 45 kWp + 30 kWh Speicher",
		CompanyID:   comp1ID,
		ContactID:   cont1ID,
		Value:       pgtype.Numeric{Int: big.NewInt(5200000), Exp: -2, Valid: true},
		Currency:    "EUR",
		Stage:       "OFFER_SENT",
		Probability: 75,
		AssignedTo:  adminID,
		CreatedAt:   nowTimestamptz(),
		UpdatedAt:   nowTimestamptz(),
	}
	q.deals[uuidToStr(deal2ID)] = db.Deal{
		ID:          deal2ID,
		Title:       "PV-Einfamilienhaus 12 kWp",
		ContactID:   cont2ID,
		Value:       pgtype.Numeric{Int: big.NewInt(1850000), Exp: -2, Valid: true},
		Currency:    "EUR",
		Stage:       "LEAD",
		Probability: 30,
		AssignedTo:  vertriebID,
		CreatedAt:   nowTimestamptz(),
		UpdatedAt:   nowTimestamptz(),
	}

	// Todos
	todo1ID := parseUUID("66666666-6666-6666-6666-666666666661")
	todo2ID := parseUUID("66666666-6666-6666-6666-666666666662")
	q.todos[uuidToStr(todo1ID)] = db.Todo{
		ID:          todo1ID,
		Title:       "Netzverträglichkeitsprüfung Netze BW einreichen",
		Description: strToText("Unterlagen für Weber Maschinenbau (45 kWp) vollständig zusammenstellen"),
		DueDate:     pgtype.Timestamptz{Time: time.Now().Add(48 * time.Hour), Valid: true},
		Status:      "OPEN",
		Priority:    "HIGH",
		AssignedTo:  adminID,
		ContactID:   cont1ID,
		DealID:      deal1ID,
		CreatedAt:   nowTimestamptz(),
		UpdatedAt:   nowTimestamptz(),
	}
	q.todos[uuidToStr(todo2ID)] = db.Todo{
		ID:          todo2ID,
		Title:       "Angebot für Sabine Mustermann nachfassen",
		Description: strToText("Telefonischer Kontakt nach Ersttermin"),
		DueDate:     pgtype.Timestamptz{Time: time.Now().Add(24 * time.Hour), Valid: true},
		Status:      "OPEN",
		Priority:    "MEDIUM",
		AssignedTo:  vertriebID,
		ContactID:   cont2ID,
		DealID:      deal2ID,
		CreatedAt:   nowTimestamptz(),
		UpdatedAt:   nowTimestamptz(),
	}

	// Notification
	notif1ID := parseUUID("77777777-7777-7777-7777-777777777771")
	q.notifications[uuidToStr(notif1ID)] = db.Notification{
		ID:        notif1ID,
		UserID:    adminID,
		Type:      "SYSTEM",
		Title:     "Willkommen im OpenLocalCRM Demo-Modus",
		Message:   "Alle Daten laufen autark im RAM. Änderungen werden isoliert ausgeführt.",
		IsRead:    false,
		CreatedAt: nowTimestamptz(),
	}

	return q
}

// User methods
func (q *InMemoryQuerier) CountUsers(ctx context.Context) (int64, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return int64(len(q.users)), nil
}

func (q *InMemoryQuerier) CreateUser(ctx context.Context, arg db.CreateUserParams) (db.User, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, u := range q.users {
		if strings.EqualFold(u.Email, arg.Email) {
			return db.User{}, errors.New("user with this email already exists")
		}
	}

	u := db.User{
		ID:           newUUID(),
		Email:        arg.Email,
		PasswordHash: arg.PasswordHash,
		FirstName:    arg.FirstName,
		LastName:     arg.LastName,
		Role:         arg.Role,
		Status:       arg.Status,
		CreatedAt:    nowTimestamptz(),
		UpdatedAt:    nowTimestamptz(),
	}
	q.users[uuidToStr(u.ID)] = u
	return u, nil
}

func (q *InMemoryQuerier) GetUserByEmail(ctx context.Context, email string) (db.User, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	for _, u := range q.users {
		if strings.EqualFold(u.Email, email) {
			return u, nil
		}
	}

	// Legacy alias compatibility: admin@mavalio.local / vertrieb@mavalio.local
	if strings.EqualFold(email, "admin@mavalio.local") {
		for _, u := range q.users {
			if strings.EqualFold(u.Email, "admin@openlocalcrm.local") {
				return u, nil
			}
		}
	}
	if strings.EqualFold(email, "vertrieb@mavalio.local") {
		for _, u := range q.users {
			if strings.EqualFold(u.Email, "vertrieb@openlocalcrm.local") {
				return u, nil
			}
		}
	}

	return db.User{}, ErrNotFound
}

func (q *InMemoryQuerier) GetUserByID(ctx context.Context, id pgtype.UUID) (db.User, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	u, ok := q.users[uuidToStr(id)]
	if !ok {
		return db.User{}, ErrNotFound
	}
	return u, nil
}

func (q *InMemoryQuerier) ListUsers(ctx context.Context) ([]db.User, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var list []db.User
	for _, u := range q.users {
		list = append(list, u)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.Time.Before(list[j].CreatedAt.Time)
	})
	return list, nil
}

func (q *InMemoryQuerier) UpdateUserLastLogin(ctx context.Context, arg db.UpdateUserLastLoginParams) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	idStr := uuidToStr(arg.ID)
	u, ok := q.users[idStr]
	if !ok {
		return ErrNotFound
	}
	u.LastLoginAt = arg.LastLoginAt
	u.UpdatedAt = nowTimestamptz()
	q.users[idStr] = u
	return nil
}

func (q *InMemoryQuerier) UpdateUserPassword(ctx context.Context, arg db.UpdateUserPasswordParams) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	idStr := uuidToStr(arg.ID)
	u, ok := q.users[idStr]
	if !ok {
		return ErrNotFound
	}
	u.PasswordHash = arg.PasswordHash
	u.UpdatedAt = nowTimestamptz()
	q.users[idStr] = u
	return nil
}

func (q *InMemoryQuerier) UpdateUserRole(ctx context.Context, arg db.UpdateUserRoleParams) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	idStr := uuidToStr(arg.ID)
	u, ok := q.users[idStr]
	if !ok {
		return ErrNotFound
	}
	u.Role = arg.Role
	u.UpdatedAt = nowTimestamptz()
	q.users[idStr] = u
	return nil
}

func (q *InMemoryQuerier) UpdateUserStatus(ctx context.Context, arg db.UpdateUserStatusParams) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	idStr := uuidToStr(arg.ID)
	u, ok := q.users[idStr]
	if !ok {
		return ErrNotFound
	}
	u.Status = arg.Status
	u.UpdatedAt = nowTimestamptz()
	q.users[idStr] = u
	return nil
}

func (q *InMemoryQuerier) UpdateUserTOTP(ctx context.Context, arg db.UpdateUserTOTPParams) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	idStr := uuidToStr(arg.ID)
	u, ok := q.users[idStr]
	if !ok {
		return ErrNotFound
	}
	u.TotpSecretEncrypted = arg.TotpSecretEncrypted
	u.TotpEnabled = arg.TotpEnabled
	u.UpdatedAt = nowTimestamptz()
	q.users[idStr] = u
	return nil
}

// Company methods
func (q *InMemoryQuerier) CreateCompany(ctx context.Context, arg db.CreateCompanyParams) (db.Company, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	c := db.Company{
		ID:             newUUID(),
		Name:           arg.Name,
		Domain:         arg.Domain,
		Phone:          arg.Phone,
		Email:          arg.Email,
		AddressStreet:  arg.AddressStreet,
		AddressZip:     arg.AddressZip,
		AddressCity:    arg.AddressCity,
		AddressCountry: arg.AddressCountry,
		CustomFields:   arg.CustomFields,
		CreatedAt:      nowTimestamptz(),
		UpdatedAt:      nowTimestamptz(),
	}
	q.companies[uuidToStr(c.ID)] = c
	return c, nil
}

func (q *InMemoryQuerier) DeleteCompany(ctx context.Context, id pgtype.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.companies, uuidToStr(id))
	return nil
}

func (q *InMemoryQuerier) GetCompanyByID(ctx context.Context, id pgtype.UUID) (db.Company, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	c, ok := q.companies[uuidToStr(id)]
	if !ok {
		return db.Company{}, ErrNotFound
	}
	return c, nil
}

func (q *InMemoryQuerier) ListCompanies(ctx context.Context, arg db.ListCompaniesParams) ([]db.Company, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var list []db.Company
	for _, c := range q.companies {
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})

	start := int(arg.Offset)
	if start > len(list) {
		return []db.Company{}, nil
	}
	list = list[start:]
	if int(arg.Limit) > 0 && len(list) > int(arg.Limit) {
		list = list[:arg.Limit]
	}
	return list, nil
}

func (q *InMemoryQuerier) SearchCompanies(ctx context.Context, arg db.SearchCompaniesParams) ([]db.Company, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	query := strings.ToLower(arg.Column1.String)
	var list []db.Company
	for _, c := range q.companies {
		if strings.Contains(strings.ToLower(c.Name), query) ||
			strings.Contains(strings.ToLower(c.Domain.String), query) ||
			strings.Contains(strings.ToLower(c.AddressCity.String), query) {
			list = append(list, c)
		}
	}
	if int(arg.Limit) > 0 && len(list) > int(arg.Limit) {
		list = list[:arg.Limit]
	}
	return list, nil
}

func (q *InMemoryQuerier) UpdateCompany(ctx context.Context, arg db.UpdateCompanyParams) (db.Company, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	idStr := uuidToStr(arg.ID)
	c, ok := q.companies[idStr]
	if !ok {
		return db.Company{}, ErrNotFound
	}
	c.Name = arg.Name
	c.Domain = arg.Domain
	c.Phone = arg.Phone
	c.Email = arg.Email
	c.AddressStreet = arg.AddressStreet
	c.AddressZip = arg.AddressZip
	c.AddressCity = arg.AddressCity
	c.AddressCountry = arg.AddressCountry
	c.CustomFields = arg.CustomFields
	c.UpdatedAt = nowTimestamptz()
	q.companies[idStr] = c
	return c, nil
}

// Contact methods
func (q *InMemoryQuerier) CreateContact(ctx context.Context, arg db.CreateContactParams) (db.Contact, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	c := db.Contact{
		ID:            newUUID(),
		CompanyID:     arg.CompanyID,
		FirstName:     arg.FirstName,
		LastName:      arg.LastName,
		Email:         arg.Email,
		Phone:         arg.Phone,
		Mobile:        arg.Mobile,
		Position:      arg.Position,
		LeadSource:    arg.LeadSource,
		AddressStreet: arg.AddressStreet,
		AddressZip:    arg.AddressZip,
		AddressCity:   arg.AddressCity,
		Latitude:      arg.Latitude,
		Longitude:     arg.Longitude,
		CustomFields:  arg.CustomFields,
		CreatedAt:     nowTimestamptz(),
		UpdatedAt:     nowTimestamptz(),
	}
	q.contacts[uuidToStr(c.ID)] = c
	return c, nil
}

func (q *InMemoryQuerier) DeleteContact(ctx context.Context, id pgtype.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.contacts, uuidToStr(id))
	return nil
}

func (q *InMemoryQuerier) GetContactByID(ctx context.Context, id pgtype.UUID) (db.Contact, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	c, ok := q.contacts[uuidToStr(id)]
	if !ok {
		return db.Contact{}, ErrNotFound
	}
	return c, nil
}

func (q *InMemoryQuerier) ListContacts(ctx context.Context, arg db.ListContactsParams) ([]db.Contact, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var list []db.Contact
	for _, c := range q.contacts {
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].LastName == list[j].LastName {
			return list[i].FirstName < list[j].FirstName
		}
		return list[i].LastName < list[j].LastName
	})

	start := int(arg.Offset)
	if start > len(list) {
		return []db.Contact{}, nil
	}
	list = list[start:]
	if int(arg.Limit) > 0 && len(list) > int(arg.Limit) {
		list = list[:arg.Limit]
	}
	return list, nil
}

func (q *InMemoryQuerier) ListContactsByCompany(ctx context.Context, companyID pgtype.UUID) ([]db.Contact, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var list []db.Contact
	for _, c := range q.contacts {
		if c.CompanyID.Valid && companyID.Valid && c.CompanyID.Bytes == companyID.Bytes {
			list = append(list, c)
		}
	}
	return list, nil
}

func (q *InMemoryQuerier) SearchContacts(ctx context.Context, arg db.SearchContactsParams) ([]db.Contact, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	query := strings.ToLower(arg.Column1.String)
	var list []db.Contact
	for _, c := range q.contacts {
		if strings.Contains(strings.ToLower(c.FirstName), query) ||
			strings.Contains(strings.ToLower(c.LastName), query) ||
			strings.Contains(strings.ToLower(c.Email.String), query) ||
			strings.Contains(strings.ToLower(c.Phone.String), query) {
			list = append(list, c)
		}
	}
	if int(arg.Limit) > 0 && len(list) > int(arg.Limit) {
		list = list[:arg.Limit]
	}
	return list, nil
}

func (q *InMemoryQuerier) UpdateContact(ctx context.Context, arg db.UpdateContactParams) (db.Contact, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	idStr := uuidToStr(arg.ID)
	c, ok := q.contacts[idStr]
	if !ok {
		return db.Contact{}, ErrNotFound
	}
	c.CompanyID = arg.CompanyID
	c.FirstName = arg.FirstName
	c.LastName = arg.LastName
	c.Email = arg.Email
	c.Phone = arg.Phone
	c.Mobile = arg.Mobile
	c.Position = arg.Position
	c.LeadSource = arg.LeadSource
	c.AddressStreet = arg.AddressStreet
	c.AddressZip = arg.AddressZip
	c.AddressCity = arg.AddressCity
	c.Latitude = arg.Latitude
	c.Longitude = arg.Longitude
	c.CustomFields = arg.CustomFields
	c.UpdatedAt = nowTimestamptz()
	q.contacts[idStr] = c
	return c, nil
}

// Deal methods
func (q *InMemoryQuerier) CreateDeal(ctx context.Context, arg db.CreateDealParams) (db.Deal, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	d := db.Deal{
		ID:           newUUID(),
		Title:        arg.Title,
		CompanyID:    arg.CompanyID,
		ContactID:    arg.ContactID,
		Value:        arg.Value,
		Currency:     arg.Currency,
		Stage:        arg.Stage,
		Probability:  arg.Probability,
		AssignedTo:   arg.AssignedTo,
		ClosedAt:     arg.ClosedAt,
		CustomFields: arg.CustomFields,
		CreatedAt:    nowTimestamptz(),
		UpdatedAt:    nowTimestamptz(),
	}
	q.deals[uuidToStr(d.ID)] = d
	return d, nil
}

func (q *InMemoryQuerier) DeleteDeal(ctx context.Context, id pgtype.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.deals, uuidToStr(id))
	return nil
}

func (q *InMemoryQuerier) GetDealByID(ctx context.Context, id pgtype.UUID) (db.Deal, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	d, ok := q.deals[uuidToStr(id)]
	if !ok {
		return db.Deal{}, ErrNotFound
	}
	return d, nil
}

func (q *InMemoryQuerier) ListDeals(ctx context.Context, arg db.ListDealsParams) ([]db.Deal, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var list []db.Deal
	for _, d := range q.deals {
		list = append(list, d)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.Time.After(list[j].CreatedAt.Time)
	})

	start := int(arg.Offset)
	if start > len(list) {
		return []db.Deal{}, nil
	}
	list = list[start:]
	if int(arg.Limit) > 0 && len(list) > int(arg.Limit) {
		list = list[:arg.Limit]
	}
	return list, nil
}

func (q *InMemoryQuerier) ListDealsByStage(ctx context.Context, stage string) ([]db.Deal, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var list []db.Deal
	for _, d := range q.deals {
		if d.Stage == stage {
			list = append(list, d)
		}
	}
	return list, nil
}

func (q *InMemoryQuerier) UpdateDeal(ctx context.Context, arg db.UpdateDealParams) (db.Deal, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	idStr := uuidToStr(arg.ID)
	d, ok := q.deals[idStr]
	if !ok {
		return db.Deal{}, ErrNotFound
	}
	d.Title = arg.Title
	d.CompanyID = arg.CompanyID
	d.ContactID = arg.ContactID
	d.Value = arg.Value
	d.Currency = arg.Currency
	d.Stage = arg.Stage
	d.Probability = arg.Probability
	d.AssignedTo = arg.AssignedTo
	d.ClosedAt = arg.ClosedAt
	d.CustomFields = arg.CustomFields
	d.UpdatedAt = nowTimestamptz()
	q.deals[idStr] = d
	return d, nil
}

func (q *InMemoryQuerier) UpdateDealStage(ctx context.Context, arg db.UpdateDealStageParams) (db.Deal, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	idStr := uuidToStr(arg.ID)
	d, ok := q.deals[idStr]
	if !ok {
		return db.Deal{}, ErrNotFound
	}
	d.Stage = arg.Stage
	d.UpdatedAt = nowTimestamptz()
	q.deals[idStr] = d
	return d, nil
}

// Todo methods
func (q *InMemoryQuerier) CreateTodo(ctx context.Context, arg db.CreateTodoParams) (db.Todo, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	t := db.Todo{
		ID:          newUUID(),
		Title:       arg.Title,
		Description: arg.Description,
		DueDate:     arg.DueDate,
		Status:      arg.Status,
		Priority:    arg.Priority,
		AssignedTo:  arg.AssignedTo,
		ContactID:   arg.ContactID,
		DealID:      arg.DealID,
		CreatedAt:   nowTimestamptz(),
		UpdatedAt:   nowTimestamptz(),
	}
	q.todos[uuidToStr(t.ID)] = t
	return t, nil
}

func (q *InMemoryQuerier) DeleteTodo(ctx context.Context, id pgtype.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.todos, uuidToStr(id))
	return nil
}

func (q *InMemoryQuerier) GetTodoByID(ctx context.Context, id pgtype.UUID) (db.Todo, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	t, ok := q.todos[uuidToStr(id)]
	if !ok {
		return db.Todo{}, ErrNotFound
	}
	return t, nil
}

func (q *InMemoryQuerier) ListTodos(ctx context.Context, arg db.ListTodosParams) ([]db.Todo, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var list []db.Todo
	for _, t := range q.todos {
		list = append(list, t)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.Time.After(list[j].CreatedAt.Time)
	})

	start := int(arg.Offset)
	if start > len(list) {
		return []db.Todo{}, nil
	}
	list = list[start:]
	if int(arg.Limit) > 0 && len(list) > int(arg.Limit) {
		list = list[:arg.Limit]
	}
	return list, nil
}

func (q *InMemoryQuerier) ListTodosByAssignee(ctx context.Context, assignedTo pgtype.UUID) ([]db.Todo, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var list []db.Todo
	for _, t := range q.todos {
		if t.AssignedTo.Valid && assignedTo.Valid && t.AssignedTo.Bytes == assignedTo.Bytes {
			list = append(list, t)
		}
	}
	return list, nil
}

func (q *InMemoryQuerier) UpdateTodo(ctx context.Context, arg db.UpdateTodoParams) (db.Todo, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	idStr := uuidToStr(arg.ID)
	t, ok := q.todos[idStr]
	if !ok {
		return db.Todo{}, ErrNotFound
	}
	t.Title = arg.Title
	t.Description = arg.Description
	t.DueDate = arg.DueDate
	t.Status = arg.Status
	t.Priority = arg.Priority
	t.AssignedTo = arg.AssignedTo
	t.ContactID = arg.ContactID
	t.DealID = arg.DealID
	t.UpdatedAt = nowTimestamptz()
	q.todos[idStr] = t
	return t, nil
}

func (q *InMemoryQuerier) UpdateTodoStatus(ctx context.Context, arg db.UpdateTodoStatusParams) (db.Todo, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	idStr := uuidToStr(arg.ID)
	t, ok := q.todos[idStr]
	if !ok {
		return db.Todo{}, ErrNotFound
	}
	t.Status = arg.Status
	t.UpdatedAt = nowTimestamptz()
	q.todos[idStr] = t
	return t, nil
}

// Notification methods
func (q *InMemoryQuerier) CreateNotification(ctx context.Context, arg db.CreateNotificationParams) (db.Notification, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	n := db.Notification{
		ID:        newUUID(),
		UserID:    arg.UserID,
		Type:      arg.Type,
		Title:     arg.Title,
		Message:   arg.Message,
		Link:      arg.Link,
		IsRead:    false,
		CreatedAt: nowTimestamptz(),
	}
	q.notifications[uuidToStr(n.ID)] = n
	return n, nil
}

func (q *InMemoryQuerier) ListAllNotifications(ctx context.Context, arg db.ListAllNotificationsParams) ([]db.Notification, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var list []db.Notification
	for _, n := range q.notifications {
		if n.UserID.Valid && arg.UserID.Valid && n.UserID.Bytes == arg.UserID.Bytes {
			list = append(list, n)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.Time.After(list[j].CreatedAt.Time)
	})
	if int(arg.Limit) > 0 && len(list) > int(arg.Limit) {
		list = list[:arg.Limit]
	}
	return list, nil
}

func (q *InMemoryQuerier) ListUnreadNotifications(ctx context.Context, arg db.ListUnreadNotificationsParams) ([]db.Notification, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var list []db.Notification
	for _, n := range q.notifications {
		if n.UserID.Valid && arg.UserID.Valid && n.UserID.Bytes == arg.UserID.Bytes && !n.IsRead {
			list = append(list, n)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.Time.After(list[j].CreatedAt.Time)
	})
	if int(arg.Limit) > 0 && len(list) > int(arg.Limit) {
		list = list[:arg.Limit]
	}
	return list, nil
}

func (q *InMemoryQuerier) MarkAllNotificationsAsRead(ctx context.Context, userID pgtype.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for idStr, n := range q.notifications {
		if n.UserID.Valid && userID.Valid && n.UserID.Bytes == userID.Bytes {
			n.IsRead = true
			q.notifications[idStr] = n
		}
	}
	return nil
}

func (q *InMemoryQuerier) MarkNotificationAsRead(ctx context.Context, id pgtype.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	idStr := uuidToStr(id)
	if n, ok := q.notifications[idStr]; ok {
		n.IsRead = true
		q.notifications[idStr] = n
	}
	return nil
}

// Audit log methods
func (q *InMemoryQuerier) CreateAuditLog(ctx context.Context, arg db.CreateAuditLogParams) (db.AuditLog, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	log := db.AuditLog{
		ID:         newUUID(),
		UserID:     arg.UserID,
		EntityType: arg.EntityType,
		EntityID:   arg.EntityID,
		Action:     arg.Action,
		Changes:    arg.Changes,
		IpAddress:  arg.IpAddress,
		UserAgent:  arg.UserAgent,
		CreatedAt:  nowTimestamptz(),
	}
	q.auditLogs = append([]db.AuditLog{log}, q.auditLogs...)
	return log, nil
}

func (q *InMemoryQuerier) ListAuditLogsByEntity(ctx context.Context, arg db.ListAuditLogsByEntityParams) ([]db.AuditLog, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var list []db.AuditLog
	for _, l := range q.auditLogs {
		if l.EntityType == arg.EntityType && l.EntityID.Valid && arg.EntityID.Valid && l.EntityID.Bytes == arg.EntityID.Bytes {
			list = append(list, l)
		}
	}
	if int(arg.Limit) > 0 && len(list) > int(arg.Limit) {
		list = list[:arg.Limit]
	}
	return list, nil
}

func (q *InMemoryQuerier) ListRecentAuditLogs(ctx context.Context, arg db.ListRecentAuditLogsParams) ([]db.AuditLog, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	list := make([]db.AuditLog, len(q.auditLogs))
	copy(list, q.auditLogs)
	if int(arg.Limit) > 0 && len(list) > int(arg.Limit) {
		list = list[:arg.Limit]
	}
	return list, nil
}

// Refresh token methods
func (q *InMemoryQuerier) CreateRefreshToken(ctx context.Context, arg db.CreateRefreshTokenParams) (db.RefreshToken, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	t := db.RefreshToken{
		ID:        newUUID(),
		UserID:    arg.UserID,
		TokenHash: arg.TokenHash,
		UserAgent: arg.UserAgent,
		IpAddress: arg.IpAddress,
		ExpiresAt: arg.ExpiresAt,
		CreatedAt: nowTimestamptz(),
	}
	q.refreshTokens[arg.TokenHash] = t
	return t, nil
}

func (q *InMemoryQuerier) DeleteRefreshToken(ctx context.Context, tokenHash string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.refreshTokens, tokenHash)
	return nil
}

func (q *InMemoryQuerier) DeleteUserRefreshTokens(ctx context.Context, userID pgtype.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for hash, t := range q.refreshTokens {
		if t.UserID.Valid && userID.Valid && t.UserID.Bytes == userID.Bytes {
			delete(q.refreshTokens, hash)
		}
	}
	return nil
}

func (q *InMemoryQuerier) GetRefreshToken(ctx context.Context, tokenHash string) (db.RefreshToken, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	t, ok := q.refreshTokens[tokenHash]
	if !ok {
		return db.RefreshToken{}, ErrNotFound
	}
	if t.ExpiresAt.Valid && t.ExpiresAt.Time.Before(time.Now()) {
		return db.RefreshToken{}, ErrNotFound
	}
	return t, nil
}

// Email methods
func (q *InMemoryQuerier) CreateEmailAccount(ctx context.Context, arg db.CreateEmailAccountParams) (db.EmailAccount, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	acc := db.EmailAccount{
		ID:                newUUID(),
		Name:              arg.Name,
		EmailAddress:      arg.EmailAddress,
		Provider:          arg.Provider,
		ImapHost:          arg.ImapHost,
		ImapPort:          arg.ImapPort,
		SmtpHost:          arg.SmtpHost,
		SmtpPort:          arg.SmtpPort,
		Username:          arg.Username,
		PasswordEncrypted: arg.PasswordEncrypted,
		IsActive:          arg.IsActive,
		CreatedAt:         nowTimestamptz(),
		UpdatedAt:         nowTimestamptz(),
	}
	q.emailAccounts[uuidToStr(acc.ID)] = acc
	return acc, nil
}

func (q *InMemoryQuerier) CreateEmailAttachment(ctx context.Context, arg db.CreateEmailAttachmentParams) (db.EmailAttachment, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	att := db.EmailAttachment{
		ID:          newUUID(),
		MessageID:   arg.MessageID,
		Filename:    arg.Filename,
		ContentType: arg.ContentType,
		SizeBytes:   arg.SizeBytes,
		StoragePath: arg.StoragePath,
		CreatedAt:   nowTimestamptz(),
	}
	q.emailAttachs[uuidToStr(att.ID)] = att
	return att, nil
}

func (q *InMemoryQuerier) CreateEmailMessage(ctx context.Context, arg db.CreateEmailMessageParams) (db.EmailMessage, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	msg := db.EmailMessage{
		ID:              newUUID(),
		AccountID:       arg.AccountID,
		ThreadID:        arg.ThreadID,
		MessageID:       arg.MessageID,
		InReplyTo:       arg.InReplyTo,
		Direction:       arg.Direction,
		SenderEmail:     arg.SenderEmail,
		SenderName:      arg.SenderName,
		RecipientEmails: arg.RecipientEmails,
		Subject:         arg.Subject,
		BodyText:        arg.BodyText,
		BodyHtml:        arg.BodyHtml,
		ReceivedAt:      arg.ReceivedAt,
		IsRead:          arg.IsRead,
		ContactID:       arg.ContactID,
		DealID:          arg.DealID,
		CreatedAt:       nowTimestamptz(),
	}
	q.emailMessages[uuidToStr(msg.ID)] = msg
	return msg, nil
}

func (q *InMemoryQuerier) GetEmailAccountByID(ctx context.Context, id pgtype.UUID) (db.EmailAccount, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	acc, ok := q.emailAccounts[uuidToStr(id)]
	if !ok {
		return db.EmailAccount{}, ErrNotFound
	}
	return acc, nil
}

func (q *InMemoryQuerier) GetEmailMessageByID(ctx context.Context, id pgtype.UUID) (db.EmailMessage, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	msg, ok := q.emailMessages[uuidToStr(id)]
	if !ok {
		return db.EmailMessage{}, ErrNotFound
	}
	return msg, nil
}

func (q *InMemoryQuerier) ListEmailAccounts(ctx context.Context) ([]db.EmailAccount, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var list []db.EmailAccount
	for _, acc := range q.emailAccounts {
		list = append(list, acc)
	}
	return list, nil
}

func (q *InMemoryQuerier) ListEmailAttachmentsByMessage(ctx context.Context, messageID pgtype.UUID) ([]db.EmailAttachment, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var list []db.EmailAttachment
	for _, att := range q.emailAttachs {
		if att.MessageID.Valid && messageID.Valid && att.MessageID.Bytes == messageID.Bytes {
			list = append(list, att)
		}
	}
	return list, nil
}

func (q *InMemoryQuerier) ListEmailMessages(ctx context.Context, arg db.ListEmailMessagesParams) ([]db.EmailMessage, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var list []db.EmailMessage
	for _, msg := range q.emailMessages {
		list = append(list, msg)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].ReceivedAt.Time.After(list[j].ReceivedAt.Time)
	})

	start := int(arg.Offset)
	if start > len(list) {
		return []db.EmailMessage{}, nil
	}
	list = list[start:]
	if int(arg.Limit) > 0 && len(list) > int(arg.Limit) {
		list = list[:arg.Limit]
	}
	return list, nil
}

func (q *InMemoryQuerier) ListEmailMessagesByThread(ctx context.Context, threadID string) ([]db.EmailMessage, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var list []db.EmailMessage
	for _, msg := range q.emailMessages {
		if msg.ThreadID == threadID {
			list = append(list, msg)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].ReceivedAt.Time.Before(list[j].ReceivedAt.Time)
	})
	return list, nil
}

func (q *InMemoryQuerier) MarkEmailMessageRead(ctx context.Context, id pgtype.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	idStr := uuidToStr(id)
	if msg, ok := q.emailMessages[idStr]; ok {
		msg.IsRead = true
		q.emailMessages[idStr] = msg
	}
	return nil
}

func (q *InMemoryQuerier) UpdateEmailAccountLastSynced(ctx context.Context, arg db.UpdateEmailAccountLastSyncedParams) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	idStr := uuidToStr(arg.ID)
	if acc, ok := q.emailAccounts[idStr]; ok {
		acc.LastSyncedAt = arg.LastSyncedAt
		acc.UpdatedAt = nowTimestamptz()
		q.emailAccounts[idStr] = acc
	}
	return nil
}

// Compile-time check that InMemoryQuerier implements db.Querier
var _ db.Querier = (*InMemoryQuerier)(nil)
