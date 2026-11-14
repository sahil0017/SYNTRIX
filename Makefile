.PHONY: all engine runtime app bundle release open test bench run clean

all: engine runtime app

engine:
	$(MAKE) -C engine

runtime: engine
	cd runtime && go build -o build/syntrixd ./cmd/syntrixd

app:
	cd app && swift build

bundle: all
	rm -rf /tmp/SYNTRIX.app build/SYNTRIX.app
	mkdir -p /tmp/SYNTRIX.app/Contents/MacOS /tmp/SYNTRIX.app/Contents/Resources build
	cp -X app/.build/debug/SyntrixApp /tmp/SYNTRIX.app/Contents/MacOS/SyntrixApp
	cp -X runtime/build/syntrixd /tmp/SYNTRIX.app/Contents/MacOS/syntrixd
	cp -X ml/predictive_model.py /tmp/SYNTRIX.app/Contents/Resources/predictive_model.py
	cp -X ml/super_intelligence.py /tmp/SYNTRIX.app/Contents/Resources/super_intelligence.py
	cp -X ml/neural_subsystems.py /tmp/SYNTRIX.app/Contents/Resources/neural_subsystems.py
	cp -X ml/app_advisor.py /tmp/SYNTRIX.app/Contents/Resources/app_advisor.py
	cp -X ml/transformer_rl.py /tmp/SYNTRIX.app/Contents/Resources/transformer_rl.py
	cp -X app/Resources/AppIcon.icns /tmp/SYNTRIX.app/Contents/Resources/AppIcon.icns
	printf '<?xml version="1.0" encoding="UTF-8"?>\n<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">\n<plist version="1.0">\n<dict>\n\t<key>CFBundleExecutable</key><string>SyntrixApp</string>\n\t<key>CFBundleIconFile</key><string>AppIcon</string>\n\t<key>CFBundleIdentifier</key><string>com.syntrix.desktop</string>\n\t<key>CFBundleName</key><string>SYNTRIX</string>\n\t<key>CFBundlePackageType</key><string>APPL</string>\n\t<key>CFBundleShortVersionString</key><string>1.0</string>\n\t<key>LSMinimumSystemVersion</key><string>14.0</string>\n\t<key>NSHighResolutionCapable</key><true/>\n</dict>\n</plist>\n' > /tmp/SYNTRIX.app/Contents/Info.plist
	xattr -cr /tmp/SYNTRIX.app
	codesign --force --sign - /tmp/SYNTRIX.app/Contents/MacOS/syntrixd
	codesign --force --sign - /tmp/SYNTRIX.app/Contents/MacOS/SyntrixApp
	codesign --force --sign - /tmp/SYNTRIX.app
	ditto /tmp/SYNTRIX.app build/SYNTRIX.app
	rm -rf /tmp/SYNTRIX.app

release: bundle
	./scripts/package_release.sh

open: bundle
	open build/SYNTRIX.app

test:
	$(MAKE) -C engine test
	cd runtime && go test -v ./...

bench: runtime
	./scripts/benchmark.sh

run: bundle
	open build/SYNTRIX.app

clean:
	$(MAKE) -C engine clean
	rm -rf runtime/build app/.build build
