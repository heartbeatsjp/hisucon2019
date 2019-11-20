FROM python:3.6
ENV PYTHONUNBUFFERED 1
RUN mkdir -p /var/www/html/app
WORKDIR /var/www/html/app
ADD ./app/requirements.txt /var/www/html/app
RUN pip install -r requirements.txt
COPY ./app /var/www/html/app
ADD ./Docker/mysql/accesslog.txt /tmp/accesslog.txt
ADD ./Docker/mysql/bulletins.txt /tmp/bulletins.txt
ADD ./Docker/mysql/comments.txt /tmp/comments.txt
ADD ./Docker/mysql/users.txt /tmp/users.txt
ADD ./Docker/mysql/bulletins_star.txt /tmp/bulletins_star.txt
ADD ./Docker/mysql/comments_star.txt /tmp/comments_star.txt
CMD ["gunicorn", "views:app", "-b", "0.0.0.0:5000"]
