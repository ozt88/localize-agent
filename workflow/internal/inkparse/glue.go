package inkparse

// Glue handling in the ink parser.
//
// Glue ("<>") is an ink feature that joins text across boundaries without
// inserting a newline. In compiled ink JSON, glue appears as the string
// literal "<>" in container arrays.
//
// Current implementation:
// - Within-container glue: handled inline by walkFlatContent. When "<>" is
//   encountered, the glue flag is set. The next ^text element is appended
//   directly to the current text buffer without a newline separator.
//
// - Cross-divert glue (D-19): when glue precedes a divert, the text from
//   the divert target should be joined to the current block. This occurs
//   in only 10 of 286 files (34 total markers). This is tracked for
//   future enhancement if needed — the current parser handles the common
//   case of within-container glue correctly.
//
// References:
// - D-16: glue is "<>" string in compiled JSON
// - D-17: glue joins text without newline
// - D-18: 10 files, 34 occurrences
// - D-19: cross-divert glue requires divert following
