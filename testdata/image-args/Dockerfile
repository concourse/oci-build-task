ARG first_image
ARG second_image

FROM ${first_image} AS first
COPY Dockerfile /Dockerfile.first

FROM ${second_image}
USER banana
ENV PATH=/darkness
ENV BA=nana
COPY Dockerfile /Dockerfile.second
