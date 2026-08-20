// Package attachments implements upload, authorized download and message
// linkage for mail attachments. Content lives in object storage; PostgreSQL
// holds metadata and ownership.
package attachments

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/httpx"
	"github.com/yelnurq/email-server/internal/storage"
)

// MaxAttachmentSize caps a single uploaded file (25 MiB).
const MaxAttachmentSize = 25 << 20

type Handlers struct {
	Pool  *pgxpool.Pool
	Store storage.ObjectStore
	Log   *slog.Logger
}

func newPublicID() string {
	var raw [15]byte
	_, _ = rand.Read(raw[:])
	return "att_" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw[:]))
}

// safeFilename strips any path components and control characters and bounds
// the length; empty results fall back to "attachment".
func safeFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == 0x7f || r == '"' {
			continue
		}
		b.WriteRune(r)
	}
	s := strings.TrimSpace(b.String())
	if s == "" || s == "." || s == ".." {
		s = "attachment"
	}
	if len(s) > 255 {
		s = s[len(s)-255:]
	}
	return s
}

// Upload stages a file: POST multipart/form-data with field "file".
func (h *Handlers) Upload(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, MaxAttachmentSize+64<<10)
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_UPLOAD", "Send a multipart form with a 'file' field (max 25 MiB)")
		return
	}
	defer file.Close()

	filename := safeFilename(header.Filename)

	// Detect content type from content, never trust the filename alone.
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	head = head[:n]
	contentType := http.DetectContentType(head)
	if contentType == "application/octet-stream" {
		if byExt := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename))); byExt != "" {
			contentType = byExt
		}
	}

	storageKey := fmt.Sprintf("attachments/%s/%s", id.TenantID, uuid.NewString())
	hasher := sha256.New()
	reader := io.TeeReader(io.MultiReader(strings.NewReader(string(head)), file), hasher)

	if err := h.Store.Put(r.Context(), storageKey, reader, -1, contentType); err != nil {
		h.Log.Error("attachment store failed", slog.String("error", err.Error()))
		httpx.Error(w, r, http.StatusBadGateway, "STORAGE_UNAVAILABLE", "Could not store the file")
		return
	}
	// Size is known only after streaming; MinIO tracked it, but we also know
	// via the hasher-wrapped reader? TeeReader does not count; use header size
	// when provided and non-negative, else stat via a second pass is avoided:
	size := header.Size

	publicID := newPublicID()
	var dbID string
	err = h.Pool.QueryRow(r.Context(), `
		INSERT INTO attachments (public_id, tenant_id, uploader_user_id, storage_key,
		                         filename, content_type, size_bytes, checksum_sha256)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`,
		publicID, id.TenantID, id.UserID, storageKey, filename, contentType, size,
		hex.EncodeToString(hasher.Sum(nil))).Scan(&dbID)
	if err != nil {
		_ = h.Store.Delete(r.Context(), storageKey)
		h.Log.Error("attachment insert failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"id":           publicID,
		"filename":     filename,
		"content_type": contentType,
		"size_bytes":   size,
	})
}

// Download streams an attachment the caller is authorized to read: they
// uploaded it (staged) or hold a mailbox copy of its message. Cross-tenant
// access is impossible (tenant filter) and unauthorized ids return 404.
func (h *Handlers) Download(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	publicID := chi.URLParam(r, "id")

	var storageKey, filename, contentType string
	var size int64
	err := h.Pool.QueryRow(r.Context(), `
		SELECT a.storage_key, a.filename, a.content_type, a.size_bytes
		FROM attachments a
		WHERE a.public_id = $1 AND a.tenant_id = $2
		  AND (
		    a.uploader_user_id = $3
		    OR EXISTS (
		      SELECT 1 FROM mailbox_messages mm
		      JOIN mailboxes mb ON mb.id = mm.mailbox_id
		      WHERE mm.message_id = a.message_id AND mb.user_id = $3
		    ) OR EXISTS (
		      SELECT 1 FROM chat_messages cm
		      JOIN chat_conversations cc ON cc.id = cm.conversation_id
		      JOIN chat_conversation_members member ON member.conversation_id = cc.id
		      WHERE cm.id = a.chat_message_id AND member.user_id = $3
		        AND cc.tenant_id = $2
		    )
		  )`,
		publicID, id.TenantID, id.UserID).
		Scan(&storageKey, &filename, &contentType, &size)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, r, http.StatusNotFound, "ATTACHMENT_NOT_FOUND", "Attachment not found")
		return
	}
	if err != nil {
		httpx.Internal(w, r)
		return
	}

	obj, err := h.Store.Get(r.Context(), storageKey)
	if err != nil {
		h.Log.Error("attachment fetch failed", slog.String("error", err.Error()))
		httpx.Error(w, r, http.StatusBadGateway, "STORAGE_UNAVAILABLE", "Could not read the file")
		return
	}
	defer obj.Close()

	w.Header().Set("Content-Type", contentType)
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.Header().Set("Content-Disposition",
		`attachment; filename="`+filename+`"; filename*=UTF-8''`+strings.ReplaceAll(strings.ReplaceAll(filename, "%", "%25"), " ", "%20"))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, obj)
}
