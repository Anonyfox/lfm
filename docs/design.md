# Design

## Generator defaults

Grounds are white, cool or neutral grey, dark, or a saturated colour from the subject; never cream, beige, ivory or other warm off-whites, and no paper, parchment or notebook styling (grain/noise overlays, ruled lines, torn edges as page decoration); the compiler fails on them.

`design.ground` checks background colours on every element, full-cover textures, and torn, zigzag or perforated gradient edges; `concept.ground` checks declared palette colours. Photos and other component imagery are outside the ground rule. `design.palette` fails the build on violet, blue and pink gradients.
