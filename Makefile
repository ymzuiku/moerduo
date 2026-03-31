.PHONY: ios ios-gen zip image voice test serve prepare-books testflight

BUNDLE_ID := com.ymzuiku.moerduo
IOS_DIR   := ios
APP_PATH  := $(IOS_DIR)/build/Build/Products/Debug-iphoneos/ListeningFirst.app

zip:
	mkdir -p $(IOS_DIR)/ListeningFirst/Resources
	rm -f $(IOS_DIR)/ListeningFirst/Resources/client.zip
	cd client && zip -r --symlinks "$(CURDIR)/$(IOS_DIR)/ListeningFirst/Resources/client.zip" . -x "*.DS_Store"
	@echo "Built client.zip"

ios: zip
	cd $(IOS_DIR) && xcodegen generate
	xcodebuild build \
		-project $(IOS_DIR)/ListeningFirst.xcodeproj \
		-scheme ListeningFirst \
		-configuration Debug \
		-arch arm64 \
		-sdk iphoneos \
		-derivedDataPath $(IOS_DIR)/build \
		-allowProvisioningUpdates \
		-quiet
	xcrun devicectl device install app --device 00008120-001259313420C01E $(APP_PATH)
	xcrun devicectl device process launch --device 00008120-001259313420C01E $(BUNDLE_ID)

API_KEYS_DIR       := ../vibe-remote-api-keys
ASC_API_KEY_ID     := 523PH2J3BK
ASC_API_ISSUER_ID  := cdc01ff3-bd77-4719-b95d-1bb10b9c14ac
ASC_KEY_FULL_PATH  := $(shell cd $(API_KEYS_DIR) 2>/dev/null && pwd)/connect_AuthKey_$(ASC_API_KEY_ID).p8

testflight: zip
	@echo "=== TestFlight Release ==="
	$(eval BUILD_NUMBER := $(shell date +%Y%m%d%H%M))
	@echo "Build number: $(BUILD_NUMBER)"
	@sed -i '' 's/CURRENT_PROJECT_VERSION: .*/CURRENT_PROJECT_VERSION: "$(BUILD_NUMBER)"/' $(IOS_DIR)/project.yml
	@if [ -n "$$KEYCHAIN_PASSWORD" ]; then \
		echo "Unlocking keychain..."; \
		security unlock-keychain -p "$$KEYCHAIN_PASSWORD" ~/Library/Keychains/login.keychain-db; \
		security set-keychain-settings -t 3600 ~/Library/Keychains/login.keychain-db; \
		security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$$KEYCHAIN_PASSWORD" ~/Library/Keychains/login.keychain-db > /dev/null 2>&1; \
	fi
	cd $(IOS_DIR) && xcodegen generate
	@echo "Archiving..."
	cd $(IOS_DIR) && xcodebuild clean archive \
		-project ListeningFirst.xcodeproj \
		-scheme ListeningFirst \
		-archivePath build/ListeningFirst.xcarchive \
		-destination 'generic/platform=iOS' \
		-allowProvisioningUpdates \
		-authenticationKeyPath "$(ASC_KEY_FULL_PATH)" \
		-authenticationKeyID "$(ASC_API_KEY_ID)" \
		-authenticationKeyIssuerID "$(ASC_API_ISSUER_ID)"
	@echo "Uploading to App Store Connect..."
	cd $(IOS_DIR) && xcodebuild -exportArchive \
		-archivePath build/ListeningFirst.xcarchive \
		-exportOptionsPlist ExportOptions.plist \
		-exportPath build/export \
		-allowProvisioningUpdates \
		-authenticationKeyPath "$(ASC_KEY_FULL_PATH)" \
		-authenticationKeyID "$(ASC_API_KEY_ID)" \
		-authenticationKeyIssuerID "$(ASC_API_ISSUER_ID)"
	@echo "=== Done! Check App Store Connect for the new build. ==="

ios-gen:
	@cd ios && xcodegen generate

image:
	@go run ./cmd/image $(PROMPT)

voice:
	@go run ./cmd/voice $(TEXT)

serve:
	@go run ./cmd/server

prepare-books:
	@go run ./cmd/prepare-books

test:
	@go test -v ./...
