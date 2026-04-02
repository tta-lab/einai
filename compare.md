
## Comparison: Session Persistence

### flicknote-agent (reference implementation)

**Strengths:**
1. **PostgreSQL-backed with JSONB** — proper atomic persistence using `messages = messages || $1::jsonb`
2. **Structured data model** — Conversation with ID, UserID, NoteID, Title, Messages, timestamps
3. **Clean role model** — Only 3 roles:
   - `user` — user messages
   - `assistant` — LLM output (with optional reasoning)
   - `result` — command output fed back to LLM (mapped to user role)
4. **Conversation management** — List, delete, find active, update title
5. **Race condition handling** — `GetOrCreateByID` handles concurrent creation
6. **Proper auth** — Every operation requires userID
7. **Soft delete** — `deleted_at IS NULL` pattern
8. **Metadata** — Timestamps + reasoning signatures from Anthropic
9. **Atomic appends** — PostgreSQL JSONB array concatenation

### einai (current implementation)

**Strengths:**
- Simple file-based JSONL persistence
- TaskID validation with taskwarrior integration

**Weaknesses:**

| Issue | einai | flicknote |
|-------|-------|-----------|
| Atomicity | O_TRUNC rewrite each save | PostgreSQL JSONB append |
| Corruption risk | High — mid-write crash corrupts session | None — atomic append |
| Roles | Has `tool` role (nonsensical) | Only user/assistant/result |
| Timestamps | None | Yes |
| Reasoning signatures | None | Yes (Anthropic) |
| Conversation mgmt | None | List, delete, find active |
| User isolation | None (just agent-task file) | Per-user with auth |
| Error recovery | Log & skip malformed lines | N/A (DB handles integrity) |

---

## On the `tool` role

You're right — `tool` makes no sense. Here's why:

1. **Fantasy/Logos semantics**: `MessageRoleTool` exists but most providers (including Anthropic) don't have a `tool` role. Tool calls are `assistant` with tool_use content blocks; **tool results are fed back as `user` messages**.

2. **flicknote handles this cleanly**:
   - Stores tool results as `result` role (explaining it's "command output")
   - Maps `result` → `fantasy.MessageRoleUser` when replaying

3. **einai's `ToFantasyMessages` bug**:
   ```go
   case "tool":
       role = fantasy.MessageRoleTool  // ← This role likely doesn't work with Anthropic
   ```
   If logos ever emits `tool` role, you should map it to `user` role (the tool result is user feedback to the LLM).

---

## Recommendations

1. **Remove `tool` role handling** — Map any tool-result-like data to `user` role or remove entirely
2. **Consider atomic append** — Instead of O_TRUNC rewrite, append only new messages (or at least write to temp file + rename)
3. **Add metadata** — Timestamps and reasoning signatures would help debugging
4. **Add timestamps to SessionMessage**:
   ```go
   type SessionMessage struct {
       Role               string `json:"role"`
       Content            string `json:"content"`
       Reasoning          string `json:"reasoning,omitempty"`
       ReasoningSignature string `json:"reasoning_signature,omitempty"`
       Timestamp          string `json:"timestamp,omitempty"`
   }
   ```
5. **Simplify role model** — Drop `tool`, adopt flicknote's `user`/`assistant`/`result` model
