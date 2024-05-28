# Dockerfile
FROM registry.access.redhat.com/ubi9/python-311@sha256:fccda5088dd13d2a3f2659e4c904beb42fc164a0c909e765f01af31c58affae3
ARG port=8080

USER root
RUN useradd -m myuser -G 0 && chmod 755 /home/myuser

COPY server /home/myuser/server
RUN chown -R myuser:myuser /home/myuser/server && chmod a+rx /home/myuser/server
RUN sed -i.bak 's/include-system-site-packages = false/include-system-site-packages = true/' /opt/app-root/pyvenv.cfg

USER myuser
WORKDIR /home/myuser
RUN mkdir /home/myuser/hf_home && chmod og+rwx /home/myuser/hf_home
RUN mkdir /home/myuser/output && chmod og+rwx /home/myuser/output
RUN mkdir /home/myuser/.cache
ENV PATH="/opt/app-root/bin:/opt/app-root/src/.local/bin/:/opt/app-root/src/bin:/home/myuser/.local/bin:/usr/local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

RUN pip install --no-cache-dir --user --upgrade ibm-generative-ai[lm-eval]
RUN pip install --no-cache-dir --user -r server/requirements.txt

# Clone the Git repository and install the Python package
RUN git clone https://github.com/EleutherAI/lm-evaluation-harness.git && \
    cd lm-evaluation-harness && \
    curl --output lm_eval/models/bam.py https://raw.githubusercontent.com/IBM/ibm-generative-ai/main/src/genai/extensions/lm_eval/model.py && \
    git apply /home/myuser/server/patch/models.patch && pip install --no-cache-dir --user -e .[unitxt]

ENV FLASK_PORT=8080
ENV PYTHONPATH=/opt/app-root/src/.local/lib/python3.11/site-packages:/home/myuser/lm-evaluation-harness:/home/myuser:/home/myuser/server
ENV HF_HOME=/home/myuser/hf_home
EXPOSE 8080

CMD ["waitress-serve", "--host", "0.0.0.0", "--port", "8080", "--call", "app:create_app"]
