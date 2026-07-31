package secondarydb

const createProfessionsTable = `CREATE TABLE IF NOT EXISTS professions (
    id serial PRIMARY KEY,
    name varchar(100) NOT NULL,
    created_at timestamp default now(),
    updated_at timestamp default now(),
    deleted_at timestamp default null
);`

const createClassesTable = `CREATE TABLE IF NOT EXISTS classes (
    id serial PRIMARY KEY,
    name varchar(100) NOT NULL,
    h_id int NOT NULL,
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
    profession_id int NOT NULL REFERENCES professions(id),
    profession_status int NOT NULL,
    meta_data jsonb default null,
    img text default null,
    created_at timestamp default now(),
    updated_at timestamp default now(),
    last_met_at timestamp default null,
    deleted_at timestamp default null
);`

// Migrate creates the professions, classes and individuals tables in the
// secondary database if they don't already exist. professions must run
// first since individuals.profession_id references it.
func Migrate() error {
	if _, err := DB.Exec(createProfessionsTable); err != nil {
		return err
	}

	if _, err := DB.Exec(createClassesTable); err != nil {
		return err
	}

	if _, err := DB.Exec(createIndividualsTable); err != nil {
		return err
	}

	return nil
}
