import os
import shutil

from fastapi import FastAPI, UploadFile, Form

app = FastAPI()


@app.post("/")
async def create_upload_file(file: UploadFile, functions: str = Form()):
    f_path = os.path.join("/tmp", file.filename)
    print("Write:", f_path)
    with open(f_path, "wb") as f:
        shutil.copyfileobj(file, f)
    return "Success"
