package note

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
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
	mu      sync.RWMutex
	querier db.Querier
	sseHub  *sse.Hub
	notes   []Note
}

func NewService(querier db.Querier, sseHub *sse.Hub) *Service {
	return &Service{
		querier: querier,
		sseHub:  sseHub,
		notes:   make([]Note, 0),
	}
}

func parseUUID(s string) pgtype.UUID {
	u, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{Valid: false}
	}
	return pgtype.UUID{Bytes: u, Valid: true}
}

func (s *Service) List(ctx context.Context, entityType, entityID string) ([]Note, error) {
	if s.querier != nil {
		var dbNotes []db.Note
		var err error
		if entityID != "" && entityType != "" {
			dbNotes, err = s.querier.ListNotesByEntity(ctx, db.ListNotesByEntityParams{
				EntityType: entityType,
				EntityID:   parseUUID(entityID),
			})
		} else {
			dbNotes, err = s.querier.ListNotes(ctx)
		}
		if err != nil {
			return nil, err
		}

		result := make([]Note, len(dbNotes))
		for i, n := range dbNotes {
			result[i] = Note{
				ID:         uuid.UUID(n.ID.Bytes).String(),
				EntityType: n.EntityType,
				EntityID:   uuid.UUID(n.EntityID.Bytes).String(),
				Type:       n.Type,
				Author:     n.Author,
				Content:    n.Content,
				CreatedAt:  n.CreatedAt.Time,
			}
		}
		return result, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Note
	for _, n := range s.notes {
		if entityID != "" && n.EntityID != "" && !strings.EqualFold(n.EntityID, entityID) {
			continue
		}
		if entityType != "" && n.EntityType != "" && !strings.EqualFold(n.EntityType, entityType) {
			continue
		}
		result = append(result, n)
	}
	if result == nil {
		result = []Note{}
	}
	return result, nil
}

func (s *Service) GetByID(ctx context.Context, id string) (Note, error) {
	if s.querier != nil {
		u, err := uuid.Parse(id)
		if err != nil {
			return Note{}, ErrNoteNotFound
		}
		n, err := s.querier.GetNoteByID(ctx, pgtype.UUID{Bytes: u, Valid: true})
		if err != nil {
			return Note{}, ErrNoteNotFound
		}
		return Note{
			ID:         uuid.UUID(n.ID.Bytes).String(),
			EntityType: n.EntityType,
			EntityID:   uuid.UUID(n.EntityID.Bytes).String(),
			Type:       n.Type,
			Author:     n.Author,
			Content:    n.Content,
			CreatedAt:  n.CreatedAt.Time,
		}, nil
	}

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

	if s.querier != nil {
		created, err := s.querier.CreateNote(ctx, db.CreateNoteParams{
			EntityType: entityType,
			EntityID:   parseUUID(input.EntityID),
			Type:       noteType,
			Author:     author,
			Content:    input.Content,
		})
		if err == nil {
			n := Note{
				ID:         uuid.UUID(created.ID.Bytes).String(),
				EntityType: created.EntityType,
				EntityID:   uuid.UUID(created.EntityID.Bytes).String(),
				Type:       created.Type,
				Author:     created.Author,
				Content:    created.Content,
				CreatedAt:  created.CreatedAt.Time,
			}
			s.broadcast("note.created", n)
			return n, nil
		}
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

	s.broadcast("note.created", n)
	return n, nil
}

func (s *Service) Update(ctx context.Context, id, content, noteType string) (Note, error) {
	if s.querier != nil {
		if u, err := uuid.Parse(id); err == nil {
			updated, err := s.querier.UpdateNote(ctx, db.UpdateNoteParams{
				ID:      pgtype.UUID{Bytes: u, Valid: true},
				Content: content,
				Type:    noteType,
			})
			if err == nil {
				n := Note{
					ID:         uuid.UUID(updated.ID.Bytes).String(),
					EntityType: updated.EntityType,
					EntityID:   uuid.UUID(updated.EntityID.Bytes).String(),
					Type:       updated.Type,
					Author:     updated.Author,
					Content:    updated.Content,
					CreatedAt:  updated.CreatedAt.Time,
				}
				s.broadcast("note.updated", n)
				return n, nil
			}
		}
	}

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
			s.broadcast("note.updated", s.notes[i])
			return s.notes[i], nil
		}
	}
	return Note{}, ErrNoteNotFound
}

func (s *Service) Delete(ctx context.Context, id string) error {
	if s.querier != nil {
		if u, err := uuid.Parse(id); err == nil {
			_ = s.querier.DeleteNote(ctx, pgtype.UUID{Bytes: u, Valid: true})
		}
		s.broadcast("note.deleted", map[string]string{"id": id})
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i, n := range s.notes {
		if n.ID == id {
			s.notes = append(s.notes[:i], s.notes[i+1:]...)
			s.broadcast("note.deleted", map[string]string{"id": id})
			return nil
		}
	}
	return nil
}

func (s *Service) broadcast(eventType string, data any) {
	if s.sseHub != nil {
		s.sseHub.Broadcast(sse.Event{
			Type: eventType,
			Data: data,
		})
	}
}
