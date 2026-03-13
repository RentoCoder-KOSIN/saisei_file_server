from fastapi import FastAPI, HTTPException, Query
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel, field_validator
import sqlite3
import os
from datetime import datetime

app = FastAPI(title="PyLab Task Server")

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)

DB_PATH = os.environ.get("DB_PATH", "/data/users.db")


def get_db():
    conn = sqlite3.connect(DB_PATH)
    conn.row_factory = sqlite3.Row
    return conn


def init_db():
    conn = get_db()
    conn.executescript("""
        CREATE TABLE IF NOT EXISTS tasks (
            id          INTEGER PRIMARY KEY AUTOINCREMENT,
            title       TEXT NOT NULL,
            description TEXT NOT NULL,
            deadline    TEXT NOT NULL,
            created_by  TEXT NOT NULL,
            created_at  TEXT NOT NULL
        );

        CREATE TABLE IF NOT EXISTS submissions (
            id           INTEGER PRIMARY KEY AUTOINCREMENT,
            task_id      INTEGER NOT NULL,
            username     TEXT NOT NULL,
            filename     TEXT NOT NULL,
            submitted_at TEXT NOT NULL,
            is_late      INTEGER NOT NULL DEFAULT 0,
            FOREIGN KEY (task_id) REFERENCES tasks(id)
        );
    """)
    conn.commit()
    conn.close()


init_db()


# ── Models ──────────────────────────────────────

class TaskCreate(BaseModel):
    title: str
    description: str
    deadline: str
    created_by: str


class SubmissionCreate(BaseModel):
    task_id: int
    username: str
    filename: str

    # GoからのPOSTで task_id が文字列で来る場合も受け付ける
    @field_validator('task_id', mode='before')
    @classmethod
    def coerce_task_id(cls, v):
        return int(v)


# ── Tasks ────────────────────────────────────────

@app.get("/tasks")
def list_tasks():
    conn = get_db()
    rows = conn.execute("SELECT * FROM tasks ORDER BY deadline ASC").fetchall()
    conn.close()

    now = datetime.now().isoformat()
    result = []
    for r in rows:
        task = dict(r)
        task["is_expired"] = r["deadline"] < now
        result.append(task)
    return result


@app.post("/tasks", status_code=201)
def create_task(body: TaskCreate):
    conn = get_db()
    user = conn.execute(
        "SELECT role FROM users WHERE username = ?", (body.created_by,)
    ).fetchone()
    if not user or user["role"] != "teacher":
        conn.close()
        raise HTTPException(status_code=403, detail="先生のみ課題を作成できます")

    now = datetime.now().isoformat()
    cur = conn.execute(
        "INSERT INTO tasks (title, description, deadline, created_by, created_at) VALUES (?,?,?,?,?)",
        (body.title, body.description, body.deadline, body.created_by, now)
    )
    conn.commit()
    task_id = cur.lastrowid
    conn.close()
    return {"id": task_id, "message": f"✅ 課題「{body.title}」を作成しました"}


@app.delete("/tasks/{task_id}")
def delete_task(task_id: int, username: str = Query(...)):
    conn = get_db()
    user = conn.execute(
        "SELECT role FROM users WHERE username = ?", (username,)
    ).fetchone()
    if not user or user["role"] != "teacher":
        conn.close()
        raise HTTPException(status_code=403, detail="先生のみ削除できます")

    conn.execute("DELETE FROM submissions WHERE task_id = ?", (task_id,))
    conn.execute("DELETE FROM tasks WHERE id = ?", (task_id,))
    conn.commit()
    conn.close()
    return {"message": "✅ 課題を削除しました"}


# ── Submissions ──────────────────────────────────

@app.post("/submissions", status_code=201)
def submit(body: SubmissionCreate):
    conn = get_db()
    task = conn.execute(
        "SELECT * FROM tasks WHERE id = ?", (body.task_id,)
    ).fetchone()
    if not task:
        conn.close()
        raise HTTPException(status_code=404, detail="課題が見つかりません")

    now = datetime.now().isoformat()
    is_late = 1 if now > task["deadline"] else 0

    conn.execute(
        "INSERT INTO submissions (task_id, username, filename, submitted_at, is_late) VALUES (?,?,?,?,?)",
        (body.task_id, body.username, body.filename, now, is_late)
    )
    conn.commit()
    conn.close()

    msg = "✅ 提出しました"
    if is_late:
        msg += "（⚠️ 期限超過）"
    return {"message": msg, "is_late": bool(is_late)}


@app.get("/submissions/mine")
def my_submissions(username: str = Query(...)):
    """自分が提出済みの task_id 一覧を返す"""
    conn = get_db()
    rows = conn.execute(
        "SELECT DISTINCT task_id FROM submissions WHERE username = ?", (username,)
    ).fetchall()
    conn.close()
    return [r["task_id"] for r in rows]


@app.get("/submissions/status/{task_id}")
def submission_status(task_id: int):
    conn = get_db()
    task = conn.execute(
        "SELECT * FROM tasks WHERE id = ?", (task_id,)
    ).fetchone()
    if not task:
        conn.close()
        raise HTTPException(status_code=404, detail="課題が見つかりません")

    all_students = conn.execute(
        "SELECT username FROM users WHERE role = 'student'"
    ).fetchall()

    submissions = conn.execute(
        "SELECT * FROM submissions WHERE task_id = ? ORDER BY submitted_at DESC",
        (task_id,)
    ).fetchall()
    conn.close()

    submitted_map = {}
    for s in submissions:
        if s["username"] not in submitted_map:
            submitted_map[s["username"]] = dict(s)

    result = []
    for stu in all_students:
        uname = stu["username"]
        if uname in submitted_map:
            s = submitted_map[uname]
            result.append({
                "username":     uname,
                "status":       "late" if s["is_late"] else "submitted",
                "submitted_at": s["submitted_at"],
                "filename":     s["filename"],
            })
        else:
            result.append({
                "username": uname,
                "status":   "pending",
                "submitted_at": None,
                "filename": None,
            })

    return {"task": dict(task), "students": result}


@app.get("/notifications/late")
def late_notifications():
    conn = get_db()
    rows = conn.execute("""
        SELECT s.*, t.title as task_title
        FROM submissions s
        JOIN tasks t ON s.task_id = t.id
        WHERE s.is_late = 1
        ORDER BY s.submitted_at DESC
    """).fetchall()
    conn.close()
    return [dict(r) for r in rows]