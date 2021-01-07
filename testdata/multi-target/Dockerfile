FROM busybox
RUN false

FROM busybox AS additional-target
LABEL target=additional-target

FROM busybox
LABEL target=final-target
