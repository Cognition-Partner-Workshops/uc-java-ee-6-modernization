package store

import (
	"context"
	"errors"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a single-row read finds nothing.
var ErrNotFound = errors.New("store: not found")

// Querier is satisfied by *pgxpool.Pool and pgx.Tx, so every read works inside or outside a transaction.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Store replaces the jdbc/TradeDataSource + EntityManager pair: one pgx pool, passed by constructor.
type Store struct {
	Pool         *pgxpool.Pool
	keyBlockSize int
	keyMutex     sync.Mutex
	keyBlocks    map[string]*keyBlock
}

type keyBlock struct{ next, limit int }

// Open connects the pool and checks the connection.
func Open(ctx context.Context, databaseURL string, maxConns, keyBlockSize int) (*Store, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	if maxConns > 0 {
		poolConfig.MaxConns = int32(maxConns)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return New(pool, keyBlockSize), nil
}

// New wraps an existing pool.
func New(pool *pgxpool.Pool, keyBlockSize int) *Store {
	if keyBlockSize <= 0 {
		keyBlockSize = 1000
	}
	return &Store{Pool: pool, keyBlockSize: keyBlockSize, keyBlocks: map[string]*keyBlock{}}
}

func (s *Store) Close() { s.Pool.Close() }

// InTx runs fn in one transaction: commit on nil, rollback on error. This is the explicit
// replacement for the EJB container's REQUIRED transaction attribute.
func (s *Store) InTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

// NextID is KeySequenceDirect.getNextID: ids come from an in-memory block; a new block of
// KEY_BLOCK_SIZE is reserved in keygenejb (SELECT ... FOR UPDATE) when the block runs out.
// Key names used by the legacy app: "account", "holding", "order".
func (s *Store) NextID(ctx context.Context, keyName string) (int, error) {
	s.keyMutex.Lock()
	defer s.keyMutex.Unlock()
	block := s.keyBlocks[keyName]
	if block == nil || block.next >= block.limit {
		start, err := s.allocateBlock(ctx, keyName)
		if err != nil {
			return 0, err
		}
		block = &keyBlock{next: start, limit: start + s.keyBlockSize}
		s.keyBlocks[keyName] = block
	}
	id := block.next
	block.next++
	return id, nil
}

// ResetKeys drops the in-memory blocks (after the tables are rebuilt or reseeded).
func (s *Store) ResetKeys() {
	s.keyMutex.Lock()
	defer s.keyMutex.Unlock()
	s.keyBlocks = map[string]*keyBlock{}
}

func (s *Store) allocateBlock(ctx context.Context, keyName string) (int, error) {
	var start int
	err := s.InTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, "SELECT keyval FROM keygenejb WHERE keyname = $1 FOR UPDATE", keyName).Scan(&start)
		if errors.Is(err, pgx.ErrNoRows) {
			start = 0
			_, err = tx.Exec(ctx, "INSERT INTO keygenejb (keyname, keyval) VALUES ($1, $2)", keyName, s.keyBlockSize)
			return err
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "UPDATE keygenejb SET keyval = $2 WHERE keyname = $1", keyName, start+s.keyBlockSize)
		return err
	})
	return start, err
}
