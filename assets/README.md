# Brand artwork

Two files, both COPIED byte for byte out of `core/packages/design/brand/`.
Nothing here is drawn, resized, recolored or composited in this repository.

| File | What it is |
| --- | --- |
| `credda-lockup-black.png` | The lockup in black, transparent, 2121x447. For **light** backgrounds. |
| `credda-lockup-white.png` | The lockup in white, transparent, 2121x447. For **dark** backgrounds. |

The lockup is the wordmark first and the mark after it, with no rule and no
divider between them. It is the form to reach for anywhere wide and horizontal,
which is what a README header is. The mark never appears on its own here.

The pair is named for the job — black-on-light, white-on-dark — and not for a
hue, because **the identity is achromatic**. There is no brand hue. Black, white
and grey say "this is Credda"; every colour on a Credda surface belongs to an
outcome. A filename with a colour in it has to be rewritten every time the
palette moves, and it has moved three times.

The mark is the **Seal**: a ring with five short notches cut into the lower-left
rim and the rest of it smooth. An append-only record where every confirmed job
is a notch and the one clean gap is where the next one goes. Credda adds a notch
only when the thing underneath it holds — reproduced, diagnosed, patched, and
proven by a test that fails before and passes after.

Its rotational symmetry is broken on purpose. Twelve evenly spaced notches with
one gap is a cog, and a cog says "machinery", which is the opposite of the
claim; what stops this being one is that the notches are a run down one side and
the rest of the rim is untouched, because the record is not finished.

## Rules

**Never hand-edit these.** They are generated output from the brand folder. To
change them, change the masters there and copy the result across again.

**Copy the pixels, do not composite them.** Pasting an RGBA image using itself
as a mask blends every partially transparent pixel toward the empty canvas, so
each antialiased edge darkens. The artwork looks identical and is not. Verify a
copy by hashing the file, not by looking at it. As of this commit both files
match their masters exactly:

```
3633802f84abf8e694230f91d782e6c60f5d99f88d88f5dc5ba7d0db2ce10f56  credda-lockup-black.png
18b61c8c5563e46cd3acac07124d60f93f3898fbba919cc04fd02968470c175f  credda-lockup-white.png
```

**Do not re-compose the lockup from the wordmark and the mark.** The gap between
them is 0.26 of the mark's ink width, measured ink edge to ink edge, and it is
baked into these files.

## Why they are committed here rather than linked from elsewhere

The README is rendered on GitHub and on pkg.go.dev, where a relative image path
does not resolve. So the tags use absolute `raw.githubusercontent.com` URLs, and
the files they point at have to live in this repository for that URL to exist.

## What was here before

`creddaseallockup{light,dark}transparent.png` — the orange/blue seal lockup from
the retired trust product. They went with it. The colours in their names are the
reason the current pair has none.
