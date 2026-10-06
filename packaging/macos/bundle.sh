#!/bin/sh
# Собирает LangSwitch.app из готового бинарника.
#
#	packaging/macos/bundle.sh <бинарник> <версия> <каталог>
set -eu

bin=$1
version=${2#v}
out=$3
here=$(dirname "$0")
app="$out/LangSwitch.app"

rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
cp "$bin" "$app/Contents/MacOS/langswitch"
sed "s/@VERSION@/$version/g" "$here/Info.plist" > "$app/Contents/Info.plist"

# Иконка .icns из assets/icon.png (256 px).
set_dir=$(mktemp -d)/icon.iconset
mkdir -p "$set_dir"
for s in 16 32 128 256; do
	sips -z $s $s "$here/../../assets/icon.png" --out "$set_dir/icon_${s}x${s}.png" >/dev/null
done
sips -z 32 32 "$here/../../assets/icon.png" --out "$set_dir/icon_16x16@2x.png" >/dev/null
sips -z 64 64 "$here/../../assets/icon.png" --out "$set_dir/icon_32x32@2x.png" >/dev/null
sips -z 256 256 "$here/../../assets/icon.png" --out "$set_dir/icon_128x128@2x.png" >/dev/null
iconutil -c icns "$set_dir" -o "$app/Contents/Resources/icon.icns"

# Ad-hoc подпись: без неё бинарник arm64 не запустится, а разрешения
# «Универсальный доступ» привязываются к подписи.
codesign --force --deep --sign - "$app"
