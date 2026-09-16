package note

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

var (
	ErrNoteNotFound = errors.New("note not found")
	ErrInvalidNote  = errors.New("note content is required")
)

type Note struct {
	ID         string    `json:"id"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	Type       string    `json:"type"`
	Author     string    `json:"author"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
}

type CreateNoteInput struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Type       string `json:"type"`
	Author     string `json:"author"`
	Content    string `json:"content"`
}

type Service struct {
	mu     sync.RWMutex
	notes  []Note
	sseHub *sse.Hub
}

func NewService(sseHub *sse.Hub) *Service {
	now := time.Now().UTC()
	initial := []Note{
		{
			ID:         "not-1",
			EntityType: "contact",
			EntityID:   "c1",
			Type:       "CALL",
			Author:     "Max Vertriebsleiter",
			Content:    "Telefonat mit Hr. Dr. Weber: Großes Interesse an 30 kWp Gewerbedach Solaranlage inkl. 20 kWh Batteriespeicher. Statikunterlagen liegen vor.",
			CreatedAt:  now.Add(-2 * time.Hour),
		},
		{
			ID:         "not-2",
			EntityType: "contact",
			EntityID:   "c1",
			Type:       "NOTE",
			Author:     "Laura Closerin",
			Content:    "Gemma 12B Analyse: Hoher gewerblicher Eigenverbrauch tagsüber durch Maschinenpark (ca. 45.000 kWh/a).",
			CreatedAt:  now.Add(-5 * time.Hour),
		},
		{
			ID:         "not-3",
			EntityType: "contact",
			EntityID:   "c2",
			Type:       "MEETING",
			Author:     "Felix Setter",
			Content:    "D2D-Erstkontakt an der Haustür: Hauseigentümerin plant PV-Anlage für 2026. Zählernummer notiert (1EMH004512998).",
			CreatedAt:  now.Add(-24 * time.Hour),
		},
	}

	return &Service{
		notes:  initial,
		sseHub: sseHub,
	}
}

func (s *Service) List(ctx context.Context, entityType, entityID string) ([]Note, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Note
	for _, n := range s.notes {
		if entityID != "" && n.EntityID != "" && n.EntityID != entityID {
			continue
		}
		if entityType != "" && n.EntityType != "" && n.EntityType != entityType {
			continue
		}
		result = append(result, n)
	}
	return result, nil
}

func (s *Service) GetByID(ctx context.Context, id string) (Note, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, n := range s.notes {
		if n.ID == id {
			return n, nil
		}
	}
	return Note{}, ErrNoteNotFound
}

func (s *Service) Create(ctx context.Context, input CreateNoteInput) (Note, error) {
	if input.Content == "" {
		return Note{}, ErrInvalidNote
	}

	noteType := input.Type
	if noteType == "" {
		noteType = "NOTE"
	}
	author := input.Author
	if author == "" {
		author = "Vertriebsmitarbeiter"
	}
	entityType := input.EntityType
	if entityType == "" {
		entityType = "contact"
	}

	n := Note{
		ID:         fmt.Sprintf("not-%s", uuid.New().String()[:8]),
		EntityType: entityType,
		EntityID:   input.EntityID,
		Type:       noteType,
		Author:     author,
		Content:    input.Content,
		CreatedAt:  time.Now().UTC(),
	}

	s.mu.Lock()
	s.notes = append([]Note{n}, s.notes...)
	s.mu.Unlock()

	if s.sseHub != nil {
		s.sseHub.Broadcast(sse.Event{
			Type: "note.created",
			Data: n,
		})
	}

	return n, nil
}

func (s *Service) Update(ctx context.Context, id, content, noteType string) (Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, n := range s.notes {
		if n.ID == id {
			if content != "" {
				s.notes[i].Content = content
			}
			if noteType != "" {
				s.notes[i].Type = noteType
			}
			return s.notes[i], nil
		}
	}
	return Note{}, ErrNoteNotFound
}

func (s *Service) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, n := range s.notes {
		if n.ID == id {
			s.notes = append(s.notes[:i], s.notes[i+1:]...)
			return nil
		}
	}
	return nil
}
