const fs = require('fs')
const path = require('path')

const targetFiles = [
  path.join(__dirname, '../node_modules/rc-slider/es/common/createSlider.js'),
  path.join(__dirname, '../node_modules/rc-slider/lib/common/createSlider.js'),
]

for (const filePath of targetFiles) {
  if (!fs.existsSync(filePath)) {
    console.warn(`[patch-rc-slider] File not found: ${filePath}, skipping...`)
    continue
  }
  let content = fs.readFileSync(filePath, 'utf8')
  let changed = false

  // 1. Patch ES version
  if (content.includes("this.onTouchUpListener = addEventListener(this.document, 'touchend', this.onEnd);") &&
      !content.includes("this.onTouchCancelListener")) {
    content = content.replace(
      "this.onTouchUpListener = addEventListener(this.document, 'touchend', this.onEnd);",
      "this.onTouchUpListener = addEventListener(this.document, 'touchend', this.onEnd);\n        this.onTouchCancelListener = addEventListener(this.document, 'touchcancel', this.onEnd);\n        this.onWindowBlurListener = addEventListener(window, 'blur', this.onEnd);"
    )
    changed = true
  }

  // 2. Patch Lib (CommonJS) version
  if (content.includes("this.onTouchUpListener = (0, _addEventListener.default)(this.document, 'touchend', this.onEnd);") &&
      !content.includes("this.onTouchCancelListener")) {
    content = content.replace(
      "this.onTouchUpListener = (0, _addEventListener.default)(this.document, 'touchend', this.onEnd);",
      "this.onTouchUpListener = (0, _addEventListener.default)(this.document, 'touchend', this.onEnd);\n        this.onTouchCancelListener = (0, _addEventListener.default)(this.document, 'touchcancel', this.onEnd);\n        this.onWindowBlurListener = (0, _addEventListener.default)(window, 'blur', this.onEnd);"
    )
    changed = true
  }

  // 3. Patch removeDocumentEvents
  if (content.includes("this.onTouchUpListener && this.onTouchUpListener.remove();") &&
      !content.includes("this.onTouchCancelListener && this.onTouchCancelListener.remove();")) {
    content = content.replace(
      "this.onTouchUpListener && this.onTouchUpListener.remove();",
      "this.onTouchUpListener && this.onTouchUpListener.remove();\n        this.onTouchCancelListener && this.onTouchCancelListener.remove();\n        this.onWindowBlurListener && this.onWindowBlurListener.remove();"
    )
    changed = true
  }

  if (changed) {
    fs.writeFileSync(filePath, content, 'utf8')
    console.log(`[patch-rc-slider] Successfully patched ${filePath}`)
  } else {
    console.log(`[patch-rc-slider] ${filePath} already patched.`)
  }
}
