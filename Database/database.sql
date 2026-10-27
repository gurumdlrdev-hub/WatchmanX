CREATE EXTENSION IF NOT EXISTS vector;


CREATE TABLE IF NOT EXISTS operators (
    id SERIAL PRIMARY KEY,
    fullname VARCHAR(100) NOT NULL,
    email VARCHAR(150) UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS face_profiles (
    id SERIAL PRIMARY KEY,
    person_name VARCHAR(100) NOT NULL,
    details TEXT,
    embedding vector(512) NOT NULL,
    image_data BYTEA,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);


CREATE INDEX IF NOT EXISTS face_profiles_embedding_idx 
ON face_profiles USING hnsw (embedding vector_cosine_ops);