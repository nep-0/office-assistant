# Immediate Duplicate Upload Warning

Uploads compute a content hash early enough to warn immediately when the same file content already exists, allowing the user to decide whether to continue. The first version does not hard-deduplicate storage; duplicate uploads still create separate document records to keep deletion, ownership, visibility, and reprocessing simple.

