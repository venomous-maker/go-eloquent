# Changelog

## v1.0.1 (2025-10-12)

Highlights
- Mongo engine nested eager-loading:
  - Support With tokens like "products.periods" that nest $lookup inside parent lookups.
  - Dotted aliases on nested relations are normalized (Mongo $lookup as cannot contain dots).
  - Nested discovery works with model-level definitions (full dotted names) or child defs using dotted LocalKey.
- Model defaults:
  - GetDefaultWith() now preloads defaults on every Query() including nested relations when ordered correctly.
- Alias matching:
  - With()/WithCount() accept either relation name or its alias; HasOne/BelongsTo/HasMany/BelongsToMany accept "name as alias".


## v1.0.0 (2025-10-12)

Highlights
- Model-level relationship definitions (Models/Base):
  - Add RelationshipType, PivotDef, RelationDef.
  - Extend MongoModel with GetRelationships and GetDefaultWith.
- Mongo engine eager-loading improvements (Engine/Mongo/Base):
  - Auto-apply model GetDefaultWith() on every Query().
  - inferRelation first consults model GetRelationships() by name or alias.
  - Alias matching fixed: With() and WithCount() accept relation name or alias.
  - HasOne/BelongsTo/HasMany/BelongsToMany accept "name as alias" tokens.
- Example models (Models/Examples):
  - Added User with profile (hasOne as main_profile) and roles (belongsToMany) and default with.

Notes
- Default eager loads will be present even if With(...) isn’t called.
- With("profile as main_profile") and With("main_profile") both work.
- Tags follow semver vX.Y.Z; this release uses v1.0.0.

Upgrade
- No breaking API changes. Existing queries continue to work; you can layer in model-level relations progressively.
