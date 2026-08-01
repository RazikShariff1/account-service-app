package secondarydb

const createProfessionsTable = `CREATE TABLE IF NOT EXISTS professions (
    id serial PRIMARY KEY,
    name varchar(100) NOT NULL,
    created_at timestamp default now(),
    updated_at timestamp default now(),
    deleted_at timestamp default null
);`

const createProfessionTypesTable = `CREATE TABLE IF NOT EXISTS profession_types (
    id serial PRIMARY KEY,
    name varchar(100) NOT NULL,
    profession_id int NOT NULL REFERENCES professions(id),
    created_at timestamp default now(),
    updated_at timestamp default now(),
    deleted_at timestamp default null
);`

const createIndividualsTable = `CREATE TABLE IF NOT EXISTS individuals (
    id serial PRIMARY KEY,
    name varchar(100) NOT NULL,
    phone varchar(15) default null,
    h_id int NOT NULL ,
    m_id int NOT NULL ,
    r_id int NOT NULL ,
    address_id int NOT NULL,
    email varchar(100) unique default null,
    profession_type_id int NOT NULL REFERENCES profession_types(id),
    profession_status int NOT NULL,
    meta_data jsonb default null,
    img text default null,
    created_at timestamp default now(),
    updated_at timestamp default now(),
    last_met_at timestamp default null,
    deleted_at timestamp default null
);`

// migrateProfessionIDToProfessionTypeID replaces individuals.profession_id
// (direct FK to professions) with profession_type_id (FK to profession_types)
// on installs that still have the old column. Existing rows are backfilled to
// profession_type_id 1 ("Engineer"), the only profession_type on hand when
// this migration was written. Guarded on the old column's existence so it's a
// no-op both on fresh installs (createIndividualsTable already creates the
// new column) and on repeated runs against an already-migrated database.
const migrateProfessionIDToProfessionTypeID = `
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'individuals' AND column_name = 'profession_id'
    ) THEN
        ALTER TABLE individuals ADD COLUMN IF NOT EXISTS profession_type_id int REFERENCES profession_types(id);
        UPDATE individuals SET profession_type_id = 1 WHERE profession_type_id IS NULL;
        ALTER TABLE individuals ALTER COLUMN profession_type_id SET NOT NULL;
        ALTER TABLE individuals DROP COLUMN profession_id;
    END IF;
END $$;`

// createActivityLogsTable stores a free-form audit trail against an
// individual: log_name identifies the kind of event (e.g. "city_updated",
// "profile_created") and old_value/new_value hold whatever JSON shape the
// caller wants to record for that event. account_id is nullable since some
// events (e.g. system-generated ones) have no acting account, and it isn't a
// foreign key: accounts lives in the primary database, not this one, so
// resolving it to full account details is done in application code (see
// activity.GetAll), not via SQL join.
const createActivityLogsTable = `CREATE TABLE IF NOT EXISTS activity_logs (
    id serial PRIMARY KEY,
    individual_id int NOT NULL REFERENCES individuals(id),
    account_id int,
    log_name varchar(100) NOT NULL,
    old_value jsonb,
    new_value jsonb,
    created_at timestamp NOT NULL DEFAULT now()
);`

// dropActivityLogsAccountDetails removes account_details from installs that
// ran an earlier version of this migration which added it — account details
// are now resolved at read time (see activity.GetAll) instead of persisted.
const dropActivityLogsAccountDetails = `ALTER TABLE activity_logs DROP COLUMN IF EXISTS account_details;`

// Migrate creates the professions, profession_types, individuals and
// activity_logs tables in the secondary database if they don't already
// exist, and migrates individuals off its old profession_id column.
// professions must run first since profession_types.profession_id
// references it, profession_types must run before individuals since
// individuals.profession_type_id references it, and individuals must run
// before activity_logs since activity_logs.individual_id references it.
func Migrate() error {
	if _, err := DB.Exec(createProfessionsTable); err != nil {
		return err
	}

	if _, err := DB.Exec(createProfessionTypesTable); err != nil {
		return err
	}

	if _, err := DB.Exec(createIndividualsTable); err != nil {
		return err
	}

	if _, err := DB.Exec(migrateProfessionIDToProfessionTypeID); err != nil {
		return err
	}

	if _, err := DB.Exec(createActivityLogsTable); err != nil {
		return err
	}

	if _, err := DB.Exec(dropActivityLogsAccountDetails); err != nil {
		return err
	}

	return nil
}
