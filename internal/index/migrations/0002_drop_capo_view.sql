-- song_state records a reader's preferences for one song. It has no capo view
-- to remember.
ALTER TABLE song_state DROP COLUMN capo_view;
