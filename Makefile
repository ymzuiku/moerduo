.PHONY: ios ios-gen zip

BUNDLE_ID := com.ymzuiku.ListeningFirst
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

ios-gen:
	@cd ios && xcodegen generate
