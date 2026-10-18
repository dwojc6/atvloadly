# Synthetic tvOS catalog

`Assets.car` was compiled from the accompanying asset catalog with Apple's
Xcode asset compiler. The original pixels are solid red. It contains a two-layer
primary icon with a nonstandard background name and an unrelated image.
No application artwork or signing material is included.

Regenerate on a Mac with Xcode:

```sh
mkdir -p /tmp/atv-icon-fixture
xcrun actool Assets.xcassets --compile /tmp/atv-icon-fixture \
  --platform appletvos --minimum-deployment-target 13.0 \
  --app-icon 'App Icon' --target-device tv \
  --output-partial-info-plist /tmp/atv-icon-fixture/info.plist
cp /tmp/atv-icon-fixture/Assets.car Assets.car
```
