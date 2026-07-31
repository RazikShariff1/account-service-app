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

// Migrate creates the professions, profession_types and individuals tables in
// the secondary database if they don't already exist, and migrates
// individuals off its old profession_id column. professions must run first
// since profession_types.profession_id references it, and profession_types
// must run before individuals since individuals.profession_type_id
// references it.
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

	return nil
}
