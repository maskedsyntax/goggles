INSERT INTO platforms (id, name, enabled, created_at) VALUES ('linkedin', 'linkedin', 1, '1970-01-01T00:00:00Z');
CREATE UNIQUE INDEX destinations_linkedin_member ON destinations(platform, external_id) WHERE platform = 'linkedin' AND external_id IS NOT NULL AND external_id != '';
