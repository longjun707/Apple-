## 批量导入账户 (Batch Account Import)

Add the ability to import multiple Apple accounts at once from text input, instead of adding them one by one.

### Changes

**Backend (`POST /api/accounts/batch`)**
- New endpoint that accepts an array of `{ appleId, password, remark }` objects (max 500)
- Validates each account, automatically skips duplicates that already exist in the database
- Returns a summary: `{ created, skipped, errors }`

**Frontend**
- New `BatchImportModal` component on the Accounts page
- Supports multiple text formats:
  - `apple_id----password` (four dashes)
  - `apple_id:password` (colon)
  - `apple_id\tpassword` (tab)
  - `apple_id password` (space)
- Comment lines starting with `#` or `//` are ignored
- Duplicate Apple IDs within the input are auto-deduplicated
- Live preview shows how many accounts were parsed before importing
- Results screen shows created / skipped / failed counts with error details

### How to use
1. Go to **Apple 账户管理** (Accounts) page
2. Click **批量导入** (Batch Import) button
3. Paste account list in any supported format
4. Click **导入** to import

_This PR was generated with [Oz](https://www.warp.dev/oz)._
