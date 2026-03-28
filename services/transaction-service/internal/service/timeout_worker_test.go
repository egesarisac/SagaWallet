package service

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
)

func TestTimeoutWorker_Helpers(t *testing.T) {
	// We instantiate a worker with nil dependencies just to test the helper methods
	worker := NewTimeoutWorker(nil, nil, nil, nil)

	t.Run("timeToPgTimestamptz", func(t *testing.T) {
		now := time.Now()
		pgTime := worker.timeToPgTimestamptz(now)
		
		assert.True(t, pgTime.Valid)
		assert.Equal(t, now, pgTime.Time)
	})

	t.Run("uuidFromPgtype", func(t *testing.T) {
		id := uuid.New()
		pgID := pgtype.UUID{Bytes: id, Valid: true}
		
		result := worker.uuidFromPgtype(pgID)
		assert.Equal(t, id.String(), result)
	})

	t.Run("uuidFromPgtype_Invalid", func(t *testing.T) {
		pgID := pgtype.UUID{Valid: false}
		
		result := worker.uuidFromPgtype(pgID)
		assert.Equal(t, "", result)
	})

	t.Run("numericToString_Invalid", func(t *testing.T) {
		num := pgtype.Numeric{Valid: false}
		
		result := worker.numericToString(num)
		assert.Equal(t, "0.00", result)
	})
}
