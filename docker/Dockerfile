# Dockerfile
FROM python:3.11-slim
ARG port=8080

RUN apt-get update && \
    apt-get install -y --no-install-recommends git curl gcc python3-dev && \
    apt-get clean

RUN useradd -m myuser

COPY server /home/myuser/server
RUN chown -R myuser:myuser /home/myuser/server && chmod a+rx /home/myuser/server

USER myuser
WORKDIR /home/myuser
RUN mkdir /home/myuser/hf_home && chmod og+rwx /home/myuser/hf_home
RUN mkdir /home/myuser/output && chmod og+rwx /home/myuser/output
RUN mkdir /home/myuser/.cache
ENV PATH="/home/myuser/.local/bin:/usr/local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

RUN pip install --no-cache-dir --user --upgrade ibm-generative-ai[lm-eval]
RUN pip install --no-cache-dir --user -r server/requirements.txt

# Clone the Git repository and install the Python package
RUN git clone https://github.com/EleutherAI/lm-evaluation-harness.git && \
    cd lm-evaluation-harness && \
    curl --output lm_eval/models/bam.py https://raw.githubusercontent.com/IBM/ibm-generative-ai/main/src/genai/extensions/lm_eval/model.py && \
    git apply /home/myuser/server/patch/models.patch && pip install --no-cache-dir --user -e .[unitxt]

ENV FLASK_PORT=8080
ENV PYTHONPATH=/home/myuser/.local/lib/python3.11/site-packages:/home/myuser/lm-evaluation-harness:/home/myuser:/home/myuser/server
ENV HF_HOME=/home/myuser/hf_home
EXPOSE 8080

CMD ["waitress-serve", "--host", "0.0.0.0", "--port", "8080", "app:app"]
