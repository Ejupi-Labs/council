# Ejupi Labs assets distributed with Council

Council uses the approved Night/Room/Chalk/Pear/Lilac/Rose/Cobalt/Muted/Error palette from
`docs/refactor/design-identity.md` and the system reading font. No font files, website source,
provider logos or invented Council artwork are distributed.

All SVG files below are copied byte-for-byte. Their shipped source directory is `ui/src/assets/`;
the build copies the same files to the embedded UI. Appearance is dark-colour for company
artwork and mono-dark for interface icons. Geometry, lettering, margins and colours are unchanged.

## Company identity artwork

Source repository: [Ejupi-Labs/ejupilabs-brand](https://github.com/Ejupi-Labs/ejupilabs-brand),
commit `0a523bf4d5902f476e5e8081f87fb53f7fe38c00`.

These are Ejupi Labs company identity assets, used with the owner's authorization. They are
not MIT-licensed generic icons. The original logo is custom vector lettering; the wordmark is
not reconstructed with a font. The logo carries the accessible name “Ejupi Labs”.

| Shipped path | Original repository path | SHA-256 |
| --- | --- | --- |
| `brand/logo-horizontal.svg` | `assets/brand/presets/dark-colour/logos/logo-horizontal.svg` | `d697a045d059366aaeafe7c2a9a8f5f4a589c0c698e5e7c32757d5b1c8e81c7a` |
| `brand/symbol.svg` | `assets/brand/presets/dark-colour/logos/symbol.svg` | `87bb971fa3b8e5f484d149b93b60fff068c6bdd161635845fe22f9ac740f7bf9` |

## Generic interface icons

Source repository: [Ejupi-Labs/ejupilabs-icons](https://github.com/Ejupi-Labs/ejupilabs-icons),
release 1.3.1, commit `c37702fc04da7a114e5eea4416d621bd7723c74d`.
Copyright (c) 2026 Ejupi Labs. MIT permission and copyright notice are retained verbatim in
[`ejupilabs-icons/LICENSE`](ejupilabs-icons/LICENSE). Each icon retains its original 48 × 48
viewBox and is scaled uniformly. Decorative images have empty alt text; controls have text names.

| Shipped path | Original repository path | SHA-256 |
| --- | --- | --- |
| `icons/edit.svg` | `mono-dark/edit.svg` | `76ca4f92ac4ac3c7ddc4987e016e7e1d1aba165cad6071a3df592fe4bdce500d` |
| `icons/file-text.svg` | `mono-dark/file-text.svg` | `19b33dd1de814469226a436d34d28fd6ca6e2b277e4c73f72c837a860109fcc7` |
| `icons/folder-open.svg` | `mono-dark/folder-open.svg` | `2000e3229359e5cda11dfdca5d1fd2fff9cea556c3d79cef572d32ec2a90c0f9` |
| `icons/plug.svg` | `mono-dark/plug.svg` | `55634bf6d59a3b69ef1c0c6cd0674f6362edf13d1b24d9c2c2dd3cd24be43e7a` |
| `icons/plus.svg` | `mono-dark/plus.svg` | `569b7f24ed9d767e48fb82eeffc8cb25e54303edab333140a1958d1777d7e748` |
| `icons/refresh.svg` | `mono-dark/refresh.svg` | `beac6e89fa19599b14d9136079403ae367415a727bed8fa4fc45461beeafe47e` |
| `icons/settings.svg` | `mono-dark/settings.svg` | `03f62e2f3eadecbd4f553961cb0f039ef9fa0dd64e97939495d477e1d38dbddd` |
| `icons/shield.svg` | `mono-dark/shield.svg` | `24aec4da4503f1e4eba4fed0db3c14d0b8cf504c721996294143998bef68306d` |

Asset hashes were checked against the two local source checkouts at these exact commits on
2026-11-05. No private brand documentation or other product's app identity is shipped.
