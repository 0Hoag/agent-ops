package organization

import (
	"context"
	"net/url"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOrganizationConcurrentOwnerRemoval(t *testing.T) {
	db := integrationDB(t)
	var schema string
	require.NoError(t, db.Raw("SELECT current_schema()").Scan(&schema).Error)
	dsn, err := url.Parse(os.Getenv("ORGANIZATION_TEST_DATABASE_URL"))
	require.NoError(t, err)
	query := dsn.Query()
	query.Set("search_path", schema)
	dsn.RawQuery = query.Encode()
	parallelDB, err := gorm.Open(postgres.Open(dsn.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := parallelDB.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(2)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	ctx, repo := context.Background(), NewRepository(parallelDB)
	first, second := testAccount(t, db), testAccount(t, db)
	org, err := repo.Create(ctx, first, "Concurrent owners")
	require.NoError(t, err)
	require.NoError(t, db.Create(&Membership{ID: uuid.NewString(), OrganizationID: org.ID, AccountID: second, Role: RoleOwner}).Error)
	start, results := make(chan struct{}), make(chan error, 2)
	for _, actor := range []string{first, second} {
		go func() {
			<-start
			results <- repo.Remove(ctx, org.ID, actor, actor)
		}()
	}
	close(start)
	one, two := <-results, <-results
	if one == nil {
		require.ErrorIs(t, two, ErrLastOwner)
	} else {
		require.ErrorIs(t, one, ErrLastOwner)
		require.NoError(t, two)
	}
	var owners int64
	require.NoError(t, db.Model(&Membership{}).Where("organization_id=? AND role=?", org.ID, RoleOwner).Count(&owners).Error)
	require.EqualValues(t, 1, owners)
}
