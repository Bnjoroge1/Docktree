FROM python:3.12-alpine
ARG BUILD_ENV=dev
RUN pip install --no-cache-dir fastapi uvicorn
COPY app.py /app/app.py
WORKDIR /app
CMD ["python", "-c", "import time; time.sleep(999999)"]