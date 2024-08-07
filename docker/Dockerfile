# Dockerfile
FROM registry.access.redhat.com/ubi9/python-311@sha256:fccda5088dd13d2a3f2659e4c904beb42fc164a0c909e765f01af31c58affae3

USER root
RUN sed -i.bak 's/include-system-site-packages = false/include-system-site-packages = true/' /opt/app-root/pyvenv.cfg
COPY --chown=1001:0 server /opt/app-root/src/server

USER default
WORKDIR /opt/app-root/src
RUN mkdir /opt/app-root/src/hf_home && chmod g+rwx /opt/app-root/src/hf_home
RUN mkdir /opt/app-root/src/output && chmod g+rwx /opt/app-root/src/output
RUN mkdir /opt/app-root/src/.cache
ENV PATH="/opt/app-root/bin:/opt/app-root/src/.local/bin/:/opt/app-root/src/bin:/usr/local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

RUN pip install --no-cache-dir --user --upgrade ibm-generative-ai[lm-eval]
RUN pip install --no-cache-dir --user -r server/requirements.txt
# Clone the Git repository and install the Python package
RUN git clone https://github.com/EleutherAI/lm-evaluation-harness.git && \
    cd lm-evaluation-harness &&  git checkout 568af943e315100af3f00937bfd6947844769ab8 && \
    curl --output lm_eval/models/bam.py https://raw.githubusercontent.com/IBM/ibm-generative-ai/main/src/genai/extensions/lm_eval/model.py && \
    git apply /opt/app-root/src/server/patch/models.patch && pip install --no-cache-dir --user -e .[unitxt]

ENV PYTHONPATH=/opt/app-root/src/.local/lib/python3.11/site-packages:/opt/app-root/src/lm-evaluation-harness:/opt/app-root/src:/opt/app-root/src/server
ENV HF_HOME=/opt/app-root/src/hf_home
EXPOSE 8080

CMD ["waitress-serve", "--host", "0.0.0.0", "--port", "8080", "--call", "app:create_app"]
