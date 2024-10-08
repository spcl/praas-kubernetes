import sys

import os
import shutil
import tempfile
import zipfile

from fastapi import FastAPI, UploadFile, Form, HTTPException, status
from fastapi.responses import HTMLResponse, FileResponse
from subprocess import Popen, PIPE

from .config import AppConfig

app = FastAPI()


delimiter = "|"


@app.get("/")
async def main():
    content = """
<body>
<form action="/upload-wheel/" enctype="multipart/form-data" method="post">
<label for="function">The list of function names ('|' delimited)</label>
<input name="functions" type="text"><br>
<input name="file" type="file" multiple><br>
<input type="submit">
</form>
</body>
    """
    return HTMLResponse(content=content)


@app.get("/function-image/{function}")
async def send_function_image(function: str):
    print("Request image for function:", function)
    config = AppConfig()
    func_path = ""
    funcs = ""
    for f in os.listdir(config.storage_path):
        funcs_in_f = os.path.splitext(f)[0].split(delimiter)
        funcs = delimiter.join(funcs_in_f)
        if function in funcs_in_f:
            func_path = os.path.join(config.storage_path, f)

    if not os.path.isfile(func_path):
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND)
    return FileResponse(path=func_path, media_type="application/octet-stream", headers={"X-PRAAS-FUNCS": funcs})


def write_entry_point(dir_name, wheel_name):
    path = os.path.join(dir_name, "entrypoint")
    with open(path, "w") as f:
        f.write(wheel_name)


def write_functions(dir_name, functions):
    path = os.path.join(dir_name, "functions")
    functions = map(lambda func: func + "\n", functions)
    with open(path, "w") as f:
        f.writelines(functions)


@app.post("/upload-wheel/")
async def create_upload_file(file: UploadFile, functions: str = Form()):
    config = AppConfig()
    temp_dir, temp_path = await create_temp_dir(file)
    deps, wheel_name = get_metadata(temp_path)

    pip_target = os.path.join(temp_dir.name, functions)
    print("Install wheel:", file.filename, file=sys.stderr, flush=True)
    cmd = ["pip", "install", "--target", pip_target, temp_path]
    process = Popen(cmd, stdout=PIPE, stderr=PIPE)
    stdout, stderr = process.communicate()
    if process.returncode != 0:
        return "There was an error:\n" + stderr.decode("utf-8")

    print("Write metadata for wheel:", file.filename, file=sys.stderr, flush=True)
    write_entry_point(pip_target, wheel_name)
    write_functions(pip_target, functions.split(delimiter))
    out_path = os.path.join(config.storage_path, functions)
    print("Zip wheel:", file.filename, file=sys.stderr, flush=True)
    shutil.make_archive(out_path, "zip", pip_target)

    print("Cleanup wheel:", file.filename, file=sys.stderr, flush=True)
    temp_dir.cleanup()
    return "Success"


def generate_runner(temp_dir, wheel, config):
    name = "runner.py"
    path = os.path.join(temp_dir.name, name)
    templ = config.get_template()
    runner = templ.substitute({"wheel": wheel})
    with open(path, mode="w") as runner_file:
        runner_file.write(runner)
    return path


async def create_temp_dir(file: UploadFile):
    temp_dir = tempfile.TemporaryDirectory()
    temp_path = os.path.join(temp_dir.name, file.filename)
    with open(temp_path, "wb") as temp_file:
        temp_file.write(await file.read())
    return temp_dir, temp_path


def get_metadata(wheel_file_path):
    deps = []
    wheel_name = ""
    with zipfile.ZipFile(wheel_file_path) as archive:
        names = archive.namelist()
        for name in names:
            if name.endswith("/METADATA"):
                for line in archive.read(name).decode("utf-8").split("\n"):
                    if line.startswith("Requires-Dist:"):
                        dep = line.split(": ")[1]
                        deps.append(dep)
                    elif line.startswith("Name:"):
                        wheel_name = line.split(": ")[1]
    return deps, wheel_name
