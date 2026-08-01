CREATE TABLE profiles (
    id TEXT PRIMARY KEY CHECK (id = 'local-profile'),
    location_present INTEGER NOT NULL DEFAULT 0 CHECK (location_present IN (0, 1)),
    location_enabled INTEGER NOT NULL DEFAULT 0 CHECK (location_enabled IN (0, 1) AND location_enabled <= location_present),
    country TEXT,
    region TEXT,
    city_present INTEGER NOT NULL DEFAULT 0 CHECK (city_present IN (0, 1)),
    city_enabled INTEGER NOT NULL DEFAULT 0 CHECK (city_enabled IN (0, 1) AND city_enabled <= city_present),
    city TEXT,
    age_present INTEGER NOT NULL DEFAULT 0 CHECK (age_present IN (0, 1)),
    age_enabled INTEGER NOT NULL DEFAULT 0 CHECK (age_enabled IN (0, 1) AND age_enabled <= age_present),
    age INTEGER CHECK (age BETWEEN 0 AND 130),
    gender_present INTEGER NOT NULL DEFAULT 0 CHECK (gender_present IN (0, 1)),
    gender_enabled INTEGER NOT NULL DEFAULT 0 CHECK (gender_enabled IN (0, 1) AND gender_enabled <= gender_present),
    gender TEXT,
    updated_at_ms INTEGER NOT NULL,
    CHECK (location_present = 0 OR (length(trim(country)) > 0 AND length(trim(region)) > 0)),
    CHECK (age_present = 1 OR age IS NULL),
    CHECK (gender_present = 1 OR gender IS NULL)
);

CREATE TABLE profile_interests (
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (length(trim(name)) > 0),
    weight REAL NOT NULL CHECK (weight BETWEEN 0.0 AND 1.0),
    PRIMARY KEY (profile_id, name)
);

CREATE TABLE sources (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL CHECK (length(trim(name)) > 0),
    url TEXT NOT NULL CHECK (length(trim(url)) > 0),
    kind TEXT NOT NULL CHECK (kind IN ('feed', 'api', 'scraper')),
    enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
    content_permission TEXT NOT NULL CHECK (content_permission IN ('metadata_only', 'full_content_allowed')),
    feed_format TEXT CHECK (feed_format IN ('auto', 'rss', 'atom')),
    api_provider TEXT,
    api_page_size INTEGER CHECK (api_page_size >= 0),
    scraper_article_selector TEXT,
    scraper_title_selector TEXT,
    scraper_excerpt_selector TEXT,
    scraper_content_selector TEXT,
    scraper_policy_status TEXT NOT NULL CHECK (scraper_policy_status IN ('not_applicable', 'pending', 'approved', 'rejected')),
    scraper_terms_url TEXT,
    scraper_robots_url TEXT,
    scraper_reviewed_at_ms INTEGER,
    scraper_review_notes TEXT,
    credential_ref TEXT,
    refresh_cursor TEXT NOT NULL DEFAULT '',
    refresh_etag TEXT NOT NULL DEFAULT '',
    refresh_last_modified TEXT NOT NULL DEFAULT '',
    last_success_at_ms INTEGER,
    last_error TEXT NOT NULL DEFAULT '',
    retry_after_ms INTEGER,
    CHECK (
        (kind = 'feed' AND feed_format IS NOT NULL AND api_provider IS NULL AND scraper_article_selector IS NULL) OR
        (kind = 'api' AND api_provider IS NOT NULL AND length(trim(api_provider)) > 0 AND feed_format IS NULL AND scraper_article_selector IS NULL) OR
        (kind = 'scraper' AND scraper_article_selector IS NOT NULL AND length(trim(scraper_article_selector)) > 0 AND scraper_title_selector IS NOT NULL AND length(trim(scraper_title_selector)) > 0 AND feed_format IS NULL AND api_provider IS NULL)
    ),
    CHECK (kind = 'scraper' OR scraper_policy_status = 'not_applicable'),
    CHECK (kind != 'scraper' OR enabled = 0 OR scraper_policy_status = 'approved'),
    CHECK (scraper_policy_status != 'approved' OR scraper_reviewed_at_ms IS NOT NULL)
);

CREATE UNIQUE INDEX sources_url_unique ON sources(url);
CREATE INDEX sources_enabled_kind ON sources(enabled, kind);

CREATE TABLE profile_preferred_sources (
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    source_id TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    PRIMARY KEY (profile_id, source_id)
);

CREATE TABLE articles (
    id TEXT PRIMARY KEY,
    fingerprint TEXT NOT NULL CHECK (length(trim(fingerprint)) > 0),
    primary_source_id TEXT NOT NULL REFERENCES sources(id) ON DELETE RESTRICT,
    canonical_url TEXT NOT NULL CHECK (length(trim(canonical_url)) > 0),
    title TEXT NOT NULL CHECK (length(trim(title)) > 0),
    author TEXT NOT NULL DEFAULT '',
    published_at_ms INTEGER,
    fetched_at_ms INTEGER NOT NULL,
    excerpt TEXT NOT NULL DEFAULT '',
    full_content TEXT,
    content_permission TEXT NOT NULL CHECK (content_permission IN ('metadata_only', 'full_content_allowed')),
    language TEXT NOT NULL DEFAULT '',
    token_count INTEGER NOT NULL DEFAULT 0 CHECK (token_count >= 0),
    CHECK (content_permission = 'full_content_allowed' OR full_content IS NULL)
);

CREATE UNIQUE INDEX articles_canonical_url_unique ON articles(canonical_url);
CREATE UNIQUE INDEX articles_fingerprint_unique ON articles(fingerprint);
CREATE INDEX articles_feed_order ON articles(published_at_ms DESC, fetched_at_ms DESC, id);
CREATE INDEX articles_primary_source_feed ON articles(primary_source_id, published_at_ms DESC, id);

CREATE TABLE article_sources (
    article_id TEXT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    source_id TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    external_id TEXT NOT NULL DEFAULT '',
    first_seen_at_ms INTEGER NOT NULL,
    last_seen_at_ms INTEGER NOT NULL,
    PRIMARY KEY (article_id, source_id)
);

CREATE UNIQUE INDEX article_sources_external_id_unique
    ON article_sources(source_id, external_id) WHERE external_id != '';

CREATE TABLE article_topics (
    article_id TEXT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    topic TEXT NOT NULL CHECK (length(trim(topic)) > 0),
    PRIMARY KEY (article_id, topic)
);

CREATE TABLE article_tokens (
    article_id TEXT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    term TEXT NOT NULL CHECK (length(term) > 0),
    frequency INTEGER NOT NULL CHECK (frequency > 0),
    PRIMARY KEY (article_id, term)
);

CREATE INDEX article_tokens_term ON article_tokens(term, article_id);

CREATE TABLE library_states (
    article_id TEXT PRIMARY KEY REFERENCES articles(id) ON DELETE CASCADE,
    read_at_ms INTEGER,
    saved_at_ms INTEGER,
    hidden_at_ms INTEGER
);

CREATE INDEX library_feed_state ON library_states(hidden_at_ms, read_at_ms, saved_at_ms, article_id);

CREATE TABLE ranking_configurations (
    profile_id TEXT PRIMARY KEY REFERENCES profiles(id) ON DELETE CASCADE,
    recency_enabled INTEGER NOT NULL CHECK (recency_enabled IN (0, 1)),
    recency_weight REAL NOT NULL CHECK (recency_weight BETWEEN 0.0 AND 1.0),
    interest_enabled INTEGER NOT NULL CHECK (interest_enabled IN (0, 1)),
    interest_weight REAL NOT NULL CHECK (interest_weight BETWEEN 0.0 AND 1.0),
    source_enabled INTEGER NOT NULL CHECK (source_enabled IN (0, 1)),
    source_weight REAL NOT NULL CHECK (source_weight BETWEEN 0.0 AND 1.0),
    behavior_enabled INTEGER NOT NULL CHECK (behavior_enabled IN (0, 1)),
    behavior_weight REAL NOT NULL CHECK (behavior_weight BETWEEN 0.0 AND 1.0),
    location_enabled INTEGER NOT NULL CHECK (location_enabled IN (0, 1)),
    location_weight REAL NOT NULL CHECK (location_weight BETWEEN 0.0 AND 1.0),
    age_enabled INTEGER NOT NULL CHECK (age_enabled IN (0, 1)),
    age_weight REAL NOT NULL CHECK (age_weight BETWEEN 0.0 AND 1.0),
    gender_enabled INTEGER NOT NULL CHECK (gender_enabled IN (0, 1)),
    gender_weight REAL NOT NULL CHECK (gender_weight BETWEEN 0.0 AND 1.0),
    text_similarity_enabled INTEGER NOT NULL CHECK (text_similarity_enabled IN (0, 1)),
    text_similarity_weight REAL NOT NULL CHECK (text_similarity_weight BETWEEN 0.0 AND 1.0),
    per_demographic_cap REAL NOT NULL CHECK (per_demographic_cap BETWEEN 0.0 AND 1.0),
    total_demographic_cap REAL NOT NULL CHECK (total_demographic_cap BETWEEN per_demographic_cap AND 1.0),
    normalization_version TEXT NOT NULL
);

CREATE TABLE ranking_results (
    article_id TEXT PRIMARY KEY REFERENCES articles(id) ON DELETE CASCADE,
    score REAL NOT NULL,
    algorithm_version TEXT NOT NULL,
    calculated_at_ms INTEGER NOT NULL
);

CREATE INDEX ranking_results_feed ON ranking_results(score DESC, calculated_at_ms DESC, article_id);

CREATE TABLE ranking_contributions (
    article_id TEXT NOT NULL REFERENCES ranking_results(article_id) ON DELETE CASCADE,
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    signal TEXT NOT NULL CHECK (signal IN ('recency', 'interest', 'source_preference', 'behavior', 'location', 'age', 'gender', 'text_similarity')),
    raw_score REAL NOT NULL,
    weight REAL NOT NULL CHECK (weight BETWEEN 0.0 AND 1.0),
    weighted_score REAL NOT NULL,
    reason_code TEXT NOT NULL,
    reason_values_json TEXT NOT NULL DEFAULT '{}',
    PRIMARY KEY (article_id, ordinal)
);

CREATE TABLE refresh_runs (
    id TEXT PRIMARY KEY,
    started_at_ms INTEGER NOT NULL,
    finished_at_ms INTEGER,
    status TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'partial_success', 'failed', 'cancelled')),
    CHECK ((status = 'running' AND finished_at_ms IS NULL) OR (status != 'running' AND finished_at_ms IS NOT NULL))
);

CREATE UNIQUE INDEX refresh_runs_single_active ON refresh_runs((1)) WHERE status = 'running';
CREATE INDEX refresh_runs_history ON refresh_runs(started_at_ms DESC, id);

CREATE TABLE refresh_outcomes (
    refresh_run_id TEXT NOT NULL REFERENCES refresh_runs(id) ON DELETE CASCADE,
    source_id TEXT NOT NULL REFERENCES sources(id) ON DELETE RESTRICT,
    fetched INTEGER NOT NULL CHECK (fetched >= 0),
    inserted INTEGER NOT NULL CHECK (inserted >= 0),
    updated INTEGER NOT NULL CHECK (updated >= 0),
    skipped INTEGER NOT NULL CHECK (skipped >= 0),
    failed INTEGER NOT NULL CHECK (failed >= 0),
    error_code TEXT NOT NULL DEFAULT '',
    error_summary TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (refresh_run_id, source_id)
);
